package store

// token_version —— JWT 吊销开关(见迁移 0015)。
//
// 问题:已签发的 Bearer JWT 在 TTL(默认 24h)内无法作废。改密/重置密码只删
// sessions 表,而 authenticate 的 JWT 分支根本不查 sessions(jwt.go 生成的 jti
// 注释写着"便于服务端吊销",但全仓无人读它)。租户改完密码,别人手里的旧
// accessToken 照样能读写全部数据。
//
// 做法:token_version 单调递增,写进 JWT 载荷;每次认证都与库里的当前值比对,
// 不一致即 401。相比"逐个 jti 黑名单",它不需要额外存储、不会随签发量线性增长,
// 代价是粒度粗(一次自增吊销该租户全部 token)—— 对"改密/重置/封禁"这三个场景
// 恰好就是想要的语义。
//
// 存量行迁移时取默认 1,而迁移前签发的 JWT 没有该声明(解析为 0)→ 全部失效。
// 这是刻意的:上线这一次就把所有历史 token 一并作废,而不是留一批永远吊不掉的老 token。

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

// TenantTokenVersion 返回租户当前的 token_version(租户不存在返回 ErrNotFound)。
//
// 单独一条查询而不是塞进 Tenant 结构体:store.go 不在本文件所属范围内,
// 而 authenticate 的 JWT 分支只需要这一个标量。
func (s *Store) TenantTokenVersion(ctx context.Context, id int64) (int64, error) {
	var v int64
	err := s.db.WithContext(ctx).Model(&Tenant{}).
		Select("token_version").Where("id = ?", id).Row().Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return v, nil
}

// IncrementTokenVersion 让该租户已签发的 JWT 全部立即失效(封禁时使用)。
func (s *Store) IncrementTokenVersion(ctx context.Context, id int64) error {
	res := s.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).
		Update("token_version", gorm.Expr("token_version + 1"))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTenantPasswordAndBumpTokenVersion 在**同一事务**内写新密码并自增 token_version。
//
// 必须原子:分成两条语句的话,进程在中间崩掉就会留下"密码已改、token_version 未动"
// 的状态 —— 恰好是这次要修的那个洞(旧 JWT 继续有效)。
func (s *Store) SetTenantPasswordAndBumpTokenVersion(ctx context.Context, id int64, hash string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Tenant{}).Where("id = ?", id).Updates(map[string]any{
			"password_hash": hash,
			"token_version": gorm.Expr("token_version + 1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}
