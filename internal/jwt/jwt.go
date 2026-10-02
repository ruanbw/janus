// Package jwt 提供 API Bearer 认证所需的 JWT(HS256)签发与校验:
// 公开端点 POST /api/auth/token 签发,受保护路由的 authenticate 中间件校验。
package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer 固定签发者标识:解析时强制匹配,防止跨服务/跨环境 token 混用。
const Issuer = "janus"

// Claims JWT 载荷:业务角色(tenant/superadmin)+ 租户 token_version + 标准注册声明。
// Sub 为租户 ID(十进制字符串);Issuer 固定 "janus";ID(jti)为每次签发随机生成。
//
// TokenVersion 是**吊销开关**:签发时快照租户当前的 token_version,认证时与库里的
// 当前值比对,不一致即 401。改密/重置密码/封禁都会自增该值 → 此前签发的 token
// 立即作废,不必等 TTL 到期。没有它,jti 虽然每次唯一却从无人查过,改密后旧
// accessToken 在 TTL(默认 24h)内一直有效。
type Claims struct {
	Role         string `json:"role"`
	TokenVersion int64  `json:"tv"`
	jwt.RegisteredClaims
}

// Manager 持有 HS256 签名密钥与 token 有效期。
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager 以给定密钥与有效期构建 Manager。
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl}
}

// Issue 为租户签发 HS256 JWT,并把签发时刻的 token_version 写进载荷。
// jti 使用 crypto/rand 随机生成(参考 session.go newToken 的写法)。
func (m *Manager) Issue(tenantID int64, role string, tokenVersion int64) (string, error) {
	now := time.Now()
	claims := Claims{
		Role:         role,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   strconv.FormatInt(tenantID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			ID:        newJTI(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse 校验签名、过期时间与 issuer,任一不合法均返回 error(调用方按 401 处理)。
func (m *Manager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) {
			// 仅接受 HMAC 系列签名,防 alg 混淆攻击(如 none 算法)
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Subject == "" || claims.Role == "" {
		return nil, errors.New("token missing subject or role")
	}
	return claims, nil
}

// newJTI 生成随机 jti(每次签发唯一,便于服务端吊销/审计)。
func newJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
