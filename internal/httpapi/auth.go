package httpapi

// 02/03 — 认证:注册、邮箱验证、登录/登出、me、改密、忘记/重置密码(会话 cookie + CSRF)。

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"janus/internal/domain"
	"janus/internal/store"
)

const (
	minPasswordLen = 8
	// maxPasswordLen 是 bcrypt 的硬上限:golang.org/x/crypto/bcrypt 对超过 72 字节的
	// 密码直接返回 ErrPasswordTooLong。原先只校验下界,注册/改密/重置提交长密码
	// 会走到 GenerateFromPassword 拿 500 —— 用户看到的是"服务器错误",而不是
	// "密码太长"。这里显式挡住,返回 E_VALIDATION。
	maxPasswordLen = 72
)

// emailTokenPurposeSetup:超管首次设置密码的一次性 token(见 migrations/0015)。
//
// 为什么需要它:超管租户由 bootstrap 按 JANUS_SUPERADMIN_EMAIL 创建,初始没有密码。
// 原实现对 FirstLoginSetup 的租户**整段跳过 bcrypt**,只要知道超管邮箱(证书透明度
// 日志、DNS 记录、GitHub 泄露都能推断)就能直接换到 superadmin 会话 —— 这是整条
// 认证链上最严重的一个洞。setup token 把"知道邮箱"升级为"能读到这个邮箱的收件箱",
// 这才是超管账号应有的门槛。
const emailTokenPurposeSetup = "setup"

// dummyPasswordHash 用于不存在邮箱时的假 bcrypt 比较,抹平"账号不存在/密码错误"
// 的响应时间差,降低邮箱枚举侧信道(登录与忘记密码)。
var dummyPasswordHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("janus-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt unavailable: " + err.Error())
	}
	return h
}()

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// validPasswordLen 校验密码字节长度(bcrypt 的口径是字节,不是 rune)。
func validPasswordLen(p string) bool {
	return len(p) >= minPasswordLen && len(p) <= maxPasswordLen
}

const passwordLenMsg = "密码长度需为 8-72 字节"

// ---------- 注册 ----------

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Slug     string `json:"slug"`
}

// handleRegister 注册:校验 slug 唯一且不与既有域名 FQDN 冲突;
// 创建租户(pending)与平台默认域名;控制台 mailer 输出验证链接。
func (a *API) handleRegister(c *gin.Context) {
	if !a.rateLimit(c, a.registerRate) {
		return
	}
	var req registerReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := domain.NormalizeEmail(req.Email)
	slug := strings.TrimSpace(req.Slug)
	if !validEmail(email) {
		writeErr(c, http.StatusBadRequest, errValidation, "邮箱格式非法")
		return
	}
	if !domain.IsValidSlug(slug) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 非法(小写字母/数字开头结尾,可含连字符,1-63 字符)", map[string]string{"field": "slug"})
		return
	}
	if !validPasswordLen(req.Password) {
		writeErrDetails(c, http.StatusBadRequest, errValidation, passwordLenMsg, map[string]string{"field": "password"})
		return
	}
	// 平台保留子域不得被租户默认域名占用:app.<平台域名> 承载后台站点。
	//
	// 原实现写成 `slug+"."+platform == platform || ... == "app."+platform`,
	// 第一个条件在 slug 非空时恒假(IsValidSlug 已保证非空),是死代码;
	// 整段实际只挡住了 slug == "app"。这里改成显式判断,别再让死条件伪装成闸门。
	if slug == "app" {
		writeErrDetails(c, http.StatusBadRequest, errValidation, "slug 与平台保留域名冲突", map[string]string{"field": "slug"})
		return
	}
	ctx := c.Request.Context()

	if _, err := a.store.GetTenantByEmail(ctx, email); err == nil {
		writeErr(c, http.StatusConflict, errConflict, "邮箱已被注册")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	slug = strings.ToLower(slug)
	if _, err := a.store.GetTenantBySlug(ctx, slug); err == nil {
		writeErr(c, http.StatusConflict, errConflict, "slug 已被占用")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// slug 与既有域名 FQDN 冲突(平台默认域名或他人自有域名)
	for _, fqdn := range []string{slug + "." + a.cfg.PlatformDomain, slug} {
		exists, err := a.store.DomainFQDNExists(ctx, fqdn)
		if err != nil {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
			return
		}
		if exists {
			writeErr(c, http.StatusConflict, errConflict, "slug 与既有域名冲突")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	tenant, err := a.store.CreateTenant(ctx, email, string(hash), slug, false)
	if err != nil {
		if store.IsUniqueViolation(err) {
			writeErr(c, http.StatusConflict, errConflict, "邮箱或 slug 已被占用")
			return
		}
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if _, err := a.store.CreatePlatformDomain(ctx, tenant.ID, slug+"."+a.cfg.PlatformDomain); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 重新获取租户(含默认域名)
	if tenant, err = a.store.GetTenantByID(ctx, tenant.ID); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 验证邮件(控制台 mailer)
	token, tokenHash := newToken()
	if err := a.store.CreateEmailToken(ctx, tenant.ID, tokenHash, "verify", a.cfg.VerifyTokenTTL); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	if err := a.mailer.SendVerifyEmail(email, token); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusCreated, tenant)
}

type verifyEmailReq struct {
	Token string `json:"token"`
}

type resendVerificationReq struct {
	Email string `json:"email"`
}

// handleResendVerification 重新发送邮箱验证邮件。
//
// 存在的理由:注册流程里"建租户 → 建默认域名 → 签发验证 token → 发信"原先四步裸跑,
// 发信一失败就 500,但租户行已经落库、邮箱已被占用。用户重试拿到 409,而验证邮件
// 永远收不到 —— 没有任何自助恢复入口,只能进数据库手工改。这个端点补上那条入口。
//
// 语义与 forgot-password 一致:恒返回 202,不泄露邮箱是否存在。
// 已验证(active)的租户不发信(没有可重发的东西),但同样返回 202。
func (a *API) handleResendVerification(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req resendVerificationReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := domain.NormalizeEmail(req.Email)
	tenant, err := a.store.GetTenantByEmail(c.Request.Context(), email)
	if err == nil && tenant.Status == "pending" {
		// 同一邮箱 5 分钟最多一封:挡掉"拿已知邮箱轰炸"的滥用。
		if a.resendVerifyRate.Allow("verify:" + email) {
			token, tokenHash := newToken()
			if cerr := a.store.CreateEmailToken(c.Request.Context(), tenant.ID, tokenHash, "verify", a.cfg.VerifyTokenTTL); cerr == nil {
				if serr := a.mailer.SendVerifyEmail(email, token); serr != nil {
					log.Printf("resend verify email: %v", serr)
				}
			}
		}
	}
	writeJSON(c, http.StatusAccepted, map[string]string{"status": "accepted"})
}

// handleVerifyEmail 邮箱验证:成功后租户转 active,并触发默认域名证书预签发探活。
func (a *API) handleVerifyEmail(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req verifyEmailReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "token required")
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(c.Request.Context(), hashToken(strings.TrimSpace(req.Token)), "verify")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		} else {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		}
		return
	}
	// 已封禁租户不得凭旧验证 token 复活
	tenant, err := a.store.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		} else {
			writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		}
		return
	}
	if tenant.Status == "banned" {
		writeErr(c, http.StatusBadRequest, errValidation, "无效或已使用的验证 token")
		return
	}
	if err := a.store.VerifyTenant(c.Request.Context(), tenantID); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 触发平台默认域名证书预签发探活(active 域名由后台任务补探活,这里立即触发)
	go a.probeTenantDefaultDomain(tenantID)
	writeJSON(c, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) probeTenantDefaultDomain(tenantID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tenant, err := a.store.GetTenantByID(ctx, tenantID)
	if err != nil || tenant.DefaultDomain == "" {
		return
	}
	if domain.ProbeCert(ctx, tenant.DefaultDomain) {
		_ = a.store.SetDomainCertStatusByFQDN(ctx, tenant.DefaultDomain, "issued")
	} else {
		_ = a.store.SetDomainCertStatusByFQDN(ctx, tenant.DefaultDomain, "failed")
	}
}

// ---------- 登录 / 登出 / me ----------

// verifyCredentials 是 handleLogin 与 handleToken 共用的凭据校验(成功返回租户,
// 失败时已写好错误响应)。
//
// 顺序刻意是「先跑一次 bcrypt,再看租户状态」。原先 switch tenant.Status 排在最前,
// 于是 pending/banned 租户 ~1ms 就返回,不存在的账号却要付一次 dummy bcrypt
// (~60-100ms)—— 攻击者据此就能区分「邮箱不存在」与「存在但未验证/已封禁」,
// 配合注册接口的 409 即可枚举租户。现在无论账号处于什么状态,响应时间都落在
// 同一个量级上,而对外的 401/403 与错误文案保持不变。
//
// 超管首次设置密码(FirstLoginSetup,即超管且 password_hash 为空)不再是免密通道:
// 必须提供一次性 setup token。token 正确才继续;不正确则按冷却补发一枚 ——
// 知道超管邮箱的人只能反复触发"发到超管邮箱的邮件"(由 a.setupRate 限流),
// 拿不到 token,进不来。
func (a *API) verifyCredentials(c *gin.Context, email, password, setupToken string) (*store.Tenant, bool) {
	ctx := c.Request.Context()
	tenant, err := a.store.GetTenantByEmail(ctx, email)
	if err != nil {
		// 等时化:账号不存在也执行一次 bcrypt 比较
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
		return nil, false
	}

	setupMode := tenant.FirstLoginSetup
	pwHash, hashErr := a.store.TenantPasswordHash(ctx, tenant.ID)
	hasPassword := hashErr == nil && pwHash != nil

	// 恒定花掉一次 bcrypt:有密码就真比,没有(超管首登)就比 dummy。
	credOK := false
	if hasPassword {
		credOK = bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(password)) == nil
	} else {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
	}

	// 状态检查放在 bcrypt 之后(时序理由见函数注释),对外语义不变。
	switch tenant.Status {
	case "pending":
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱未验证,请查收验证邮件")
		return nil, false
	case "banned":
		writeErr(c, http.StatusForbidden, errForbidden, "账号已被封禁")
		return nil, false
	}

	if setupMode {
		if !a.consumeSetupToken(c, tenant, setupToken) {
			return nil, false
		}
	} else if !credOK {
		writeErr(c, http.StatusUnauthorized, errUnauth, "邮箱或密码错误")
		return nil, false
	}
	return tenant, true
}

// consumeSetupToken 校验并消费超管首登的一次性 setup token。
// 失败一律 401,并在冷却允许时补发一枚新 token(让运维丢了邮件还能自助恢复)。
func (a *API) consumeSetupToken(c *gin.Context, tenant *store.Tenant, token string) bool {
	token = strings.TrimSpace(token)
	if token != "" {
		tid, err := a.store.ConsumeEmailToken(c.Request.Context(), hashToken(token), emailTokenPurposeSetup)
		if err == nil && tid == tenant.ID {
			return true
		}
	}
	a.issueSetupToken(c, tenant)
	writeErr(c, http.StatusUnauthorized, errUnauth,
		"超管首次登录需要邮箱中的一次性 setup token(已重新发送到 "+tenant.Email+")")
	return false
}

// issueSetupToken 补发一枚超管首登 setup token。发信目标固定是租户自己的邮箱
// (即 JANUS_SUPERADMIN_EMAIL 对应的那个地址),节流键也用该邮箱。
//
// 节流是必需的:没有它,任何知道超管邮箱的人都能把它当邮件炸弹反复触发;
// 有它之后同一邮箱 15 分钟最多一封,而攻击者拿不到 token。
func (a *API) issueSetupToken(c *gin.Context, tenant *store.Tenant) {
	if !a.setupRate.Allow("setup:" + tenant.Email) {
		return
	}
	token, tokenHash := newToken()
	if err := a.store.CreateEmailToken(c.Request.Context(), tenant.ID, tokenHash,
		emailTokenPurposeSetup, a.cfg.ResetTokenTTL); err != nil {
		log.Printf("issue setup token: %v", err)
		return
	}
	// 复用重置邮件模板:setup token 与 reset token 的性质完全相同(都是"凭收件箱
	// 证明身份后设置密码"的一次性凭证),区别只在签发时机与用途标记。
	if err := a.mailer.SendResetEmail(tenant.Email, token); err != nil {
		log.Printf("send setup email: %v", err)
	}
}

type loginReq struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	SetupToken string `json:"setupToken"` // 仅超管首次设置密码时需要(见 verifyCredentials)
	RememberMe *bool  `json:"rememberMe"` // 契约调整:可选;true/省略 30 天,false 24 小时
}

func (a *API) handleLogin(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req loginReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := domain.NormalizeEmail(req.Email)
	tenant, ok := a.verifyCredentials(c, email, req.Password, req.SetupToken)
	if !ok {
		return
	}
	ttl := a.cfg.SessionTTL
	if req.RememberMe != nil && !*req.RememberMe {
		ttl = a.cfg.SessionTTLShort
	}
	token, tokenHash := newToken()
	csrf, _ := newToken()
	sess, err := a.store.CreateSession(c.Request.Context(), tenant.ID, tokenHash, csrf, ttl)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	a.setSessionCookies(c, token, sess.CSRFToken, ttl)
	writeJSON(c, http.StatusOK, tenant)
}

func (a *API) handleLogout(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	_ = t
	if ck, err := c.Request.Cookie(sessionCookieName); err == nil {
		_ = a.store.DeleteSession(c.Request.Context(), hashToken(ck.Value))
	}
	a.clearSessionCookies(c)
	writeNoContent(c)
}

func (a *API) handleMe(c *gin.Context) {
	t, _, ok := a.requireSession(c)
	if !ok {
		return
	}
	writeJSON(c, http.StatusOK, t)
}

// ---------- 修改密码 / 忘记密码 / 重置密码(03) ----------

type changePasswordReq struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// handleChangePassword 修改密码;超管首次登录(无密码)可省略旧密码。
func (a *API) handleChangePassword(c *gin.Context) {
	t, sess, ok := a.requireSession(c)
	if !ok {
		return
	}
	if !a.requireCSRF(c, sess) {
		return
	}
	var req changePasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(c, http.StatusBadRequest, errValidation, "新密码"+passwordLenMsg)
		return
	}
	if !t.FirstLoginSetup {
		pwHash, err := a.store.TenantPasswordHash(c.Request.Context(), t.ID)
		if err != nil || pwHash == nil || bcrypt.CompareHashAndPassword([]byte(*pwHash), []byte(req.OldPassword)) != nil {
			writeErr(c, http.StatusBadRequest, errValidation, "旧密码错误")
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 改密 = 自增 token_version:此前签发的 JWT(含别人手��的)立即作废,
	// 与"删 sessions 表"一起覆盖两种认证方式。同一事务,避免半更新。
	if err := a.store.SetTenantPasswordAndBumpTokenVersion(c.Request.Context(), t.ID, string(hash)); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 改密后吊销其他终端会话,保留当前会话(无会话的 Bearer 请求则吊销全部浏览器会话)。
	if sess != nil {
		_ = a.store.DeleteSessionsByTenantExcept(c.Request.Context(), t.ID, sess.TokenHash)
	} else {
		_ = a.store.DeleteSessionsByTenant(c.Request.Context(), t.ID)
	}
	writeNoContent(c)
}

type forgotPasswordReq struct {
	Email string `json:"email"`
}

// handleForgotPassword 始终返回 202,不泄露邮箱存在性。
func (a *API) handleForgotPassword(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req forgotPasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := domain.NormalizeEmail(req.Email)
	if tenant, err := a.store.GetTenantByEmail(c.Request.Context(), email); err == nil {
		// 等时化:账号存在与否都执行一次 bcrypt 比较,避免存在性被时序区分
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(email))
		token, tokenHash := newToken()
		if err := a.store.CreateEmailToken(c.Request.Context(), tenant.ID, tokenHash, "reset", a.cfg.ResetTokenTTL); err == nil {
			_ = a.mailer.SendResetEmail(email, token)
		}
	} else {
		// 等时化:邮箱不存在也执行一次 bcrypt 比较
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(email))
	}
	writeJSON(c, http.StatusAccepted, map[string]string{"status": "accepted"})
}

type resetPasswordReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// handleResetPassword 无效/已使用/过期 token 一律 400。
func (a *API) handleResetPassword(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req resetPasswordReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeErr(c, http.StatusBadRequest, errValidation, "token required")
		return
	}
	if !validPasswordLen(req.NewPassword) {
		writeErr(c, http.StatusBadRequest, errValidation, "新密码"+passwordLenMsg)
		return
	}
	tenantID, err := a.store.ConsumeEmailToken(c.Request.Context(), hashToken(strings.TrimSpace(req.Token)), "reset")
	if err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "无效、已使用或过期的重置 token")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 同上:重置密码也必须让已签发的 JWT 失效。
	if err := a.store.SetTenantPasswordAndBumpTokenVersion(c.Request.Context(), tenantID, string(hash)); err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	// 密码重置后吊销该租户全部既有会话,旧会话不得继续访问。
	_ = a.store.DeleteSessionsByTenant(c.Request.Context(), tenantID)
	writeNoContent(c)
}

// ---------- API Bearer(JWT)token 端点 ----------

type tokenReq struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	SetupToken string `json:"setupToken"` // 仅超管首次设置密码时需要
}

// tokenResp 契约:签发成功返回 accessToken / tokenType / expiresIn(秒),不写 cookie。
type tokenResp struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int64  `json:"expiresIn"`
}

// handleToken 签发 API Bearer JWT:校验逻辑与 handleLogin 完全一致
// (邮箱小写、pending→401、banned→403、错误凭据→401 等时化 dummy bcrypt),
// 成功后按租户 IsSuperAdmin 计算角色并用 JANUS_JWT_SECRET/JWTTTL 签发 HS256 JWT。
func (a *API) handleToken(c *gin.Context) {
	if !a.rateLimit(c, a.authRate) {
		return
	}
	var req tokenReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeErr(c, http.StatusBadRequest, errValidation, "invalid JSON body")
		return
	}
	email := domain.NormalizeEmail(req.Email)
	tenant, ok := a.verifyCredentials(c, email, req.Password, req.SetupToken)
	if !ok {
		return
	}
	role := "tenant"
	if tenant.IsSuperAdmin {
		role = "superadmin"
	}
	// 复用 server 构建的 jwtMgr(密钥回退逻辑一致:JANUS_JWT_SECRET 为空时用启动期随机密钥,
	// 与 authenticate 的校验密钥保持一致,避免签发的 token 被 401 拒绝)。
	tokenVersion, err := a.store.TenantTokenVersion(c.Request.Context(), tenant.ID)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	token, err := a.jwtMgr.Issue(tenant.ID, role, tokenVersion)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, errInternal, "internal error")
		return
	}
	writeJSON(c, http.StatusOK, tokenResp{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(a.cfg.JWTTTL.Seconds()),
	})
}
