package store

import (
	"context"
	"errors"
	"time"
)

type Visit struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	LinkID       int64     `json:"linkId" gorm:"column:link_id"`
	DomainID     int64     `json:"-" gorm:"column:domain_id"`
	Domain       string    `json:"domain" gorm:"->"` // 只读字段,查询时 join 填充
	IP           string    `json:"ip"`
	UserAgent    string    `json:"userAgent" gorm:"column:user_agent"`
	Referer      string    `json:"referer"`
	Action       string    `json:"action"`
	Outcome      string    `json:"outcome"`
	Reason       string    `json:"reason"`
	TargetURL    string    `json:"targetUrl" gorm:"column:target_url"`
	Country      string    `json:"country"`
	IsDatacenter bool      `json:"isDatacenter" gorm:"column:is_datacenter"`
	ASN          string    `json:"asn"`
	Lang         string    `json:"lang"`
	CreatedAt    time.Time `json:"createdAt" gorm:"column:created_at"`
}

// 访问动作:一次 visits 行代表"触发了什么动作"。
// link.visits 计数只含成功的 redirect / landing_view(见 CountVisitsByLink),
// click 是落地页按钮回传,混进访问量会把到达率算高一倍。
const (
	VisitActionRedirect    = "redirect"     // 跳转型短链的一次访问
	VisitActionLandingView = "landing_view" // 落地页型短链的一次访问(落地页视图)
	VisitActionClick       = "click"        // 落地页按钮经 SDK 回传的一次点击
)

// 动作结果:成功即按预期重定向,失败是可归属到该短链的失败(见 VisitReason*)。
const (
	VisitOutcomeSuccess = "success"
	VisitOutcomeFailed  = "failed"
)

// 失败原因:仅 outcome='failed' 时非空。
// 不记录无法归属到具体短链的失败(短码未命中、域名未激活、租户被封禁)。
const (
	VisitReasonLinkDisabled   = "link_disabled"   // 短码命中但短链已停用
	VisitReasonLinkDeleted    = "link_deleted"    // 短码命中但短链已逻辑删除
	VisitReasonNoTarget       = "no_target"       // PickTarget 失败(目标 URL 列表为空)
	VisitReasonLandingMissing = "landing_missing" // landing+upload 来源但托管文件缺失
)

// visitCountActions 计入访问量的动作集合(关键不变式的一半:click 排除在外;
// 另一半是 outcome 必须为 success,见 CountVisitsByLink)。
var visitCountActions = []string{VisitActionRedirect, VisitActionLandingView}

// ErrInvalidAction 访问列表的 action 过滤值非法(不在 VisitAction* 枚举内)。
// 与 ErrNotFound 分开定义:前者是调用方传参错误(可映射为 400),后者是数据缺失。
var ErrInvalidAction = errors.New("invalid visit action")

// ValidVisitAction 判断 action 过滤值是否合法(空串 = 不过滤,视为合法)。
// 上层用它提前回 400,ListVisitsByLink 内部仍会再校验一次作为兜底。
func ValidVisitAction(action string) bool {
	switch action {
	case "":
		return true
	case VisitActionRedirect, VisitActionLandingView, VisitActionClick:
		return true
	}
	return false
}

// validVisitAction action 过滤值白名单校验(空串表示不过滤)。
func validVisitAction(action string) error {
	if ValidVisitAction(action) {
		return nil
	}
	return ErrInvalidAction
}

// VisitRecord 一次访问/点击的入库字段(Action/Outcome/Reason/TargetURL/Lang 由调用方填写;
// Country/IsDatacenter/ASN 暂无 GeoIP 数据源,恒为占位值)。
type VisitRecord struct {
	LinkID    int64
	DomainID  int64
	IP        string
	UserAgent string
	Referer   string
	Action    string
	Outcome   string
	Reason    string
	TargetURL string
	Lang      string
}

// InsertVisit 记录一次访问/点击动作(含访问者 IP)。
// 统计写入失败由调用方决定是否阻断跳转,本函数只返回 error。
func (s *Store) InsertVisit(ctx context.Context, rec VisitRecord) error {
	v := Visit{
		LinkID: rec.LinkID, DomainID: rec.DomainID, IP: rec.IP,
		UserAgent: rec.UserAgent, Referer: rec.Referer,
		Action: rec.Action, Outcome: rec.Outcome, Reason: rec.Reason,
		TargetURL: rec.TargetURL, Lang: rec.Lang,
	}
	return s.db.WithContext(ctx).Create(&v).Error
}

// CountVisitsByLink 访问计数 = 成功的访问行(action IN ('redirect','landing_view') 且 outcome='success')。
// 两类行都不得灌水:click 是落地页按钮回传;outcome='failed' 是短链不可用导致的失败,
// 租户看到"访问量"时应理解为真正被成功重定向出去的次数。
func (s *Store) CountVisitsByLink(ctx context.Context, linkID int64) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&Visit{}).
		Where("link_id = ? AND action IN ? AND outcome = ?", linkID, visitCountActions, VisitOutcomeSuccess).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListVisitsByLink 分页访问列表(按时间倒序)。
// action 非空时按该动作过滤,取值不在枚举内返回 ErrInvalidAction(由上层映射 400)。
func (s *Store) ListVisitsByLink(ctx context.Context, linkID int64, action string, page, pageSize int) ([]*Visit, int, error) {
	if err := validVisitAction(action); err != nil {
		return nil, 0, err
	}
	// count 与明细两段查询必须用同一个动作过滤条件,否则 total 与 items 会不一致
	q := s.db.WithContext(ctx).Model(&Visit{}).Where("link_id = ?", linkID)
	itemQ := s.db.WithContext(ctx).Table("visits v").Where("v.link_id = ?", linkID)
	if action != "" {
		q = q.Where("action = ?", action)
		itemQ = itemQ.Where("v.action = ?", action)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []*Visit
	err := itemQ.
		Select("v.id, v.link_id, d.fqdn AS domain, v.ip, v.user_agent, v.referer, " +
			"v.action, v.outcome, v.reason, v.target_url, v.country, v.is_datacenter, v.asn, v.lang, v.created_at").
		Joins("JOIN domains d ON d.id = v.domain_id").
		Order("v.id DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).Scan(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

// CleanupVisitsBefore 清理保留期前的访问记录。
func (s *Store) CleanupVisitsBefore(ctx context.Context, before time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", before).Delete(&Visit{})
	return res.RowsAffected, res.Error
}
