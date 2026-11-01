package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/TuF3i/kamc/internal/platform/errcode"
	"github.com/TuF3i/kamc/internal/platform/modules"
)

// tokenGracePeriod 轮换后旧 Token 的宽限期（设计文档 §6.4，默认 24h）
const tokenGracePeriod = 24 * time.Hour

// registerTokenTTL 注册 Token 有效期（1h，设计文档 §6.1）
const registerTokenTTL = time.Hour

// ErrTokenInvalid Token 校验失败（sentinel，模式 A：供 /tunnel 入口内部判定，
// 不直接暴露给 HTTP API —— 对外一律 401，不区分原因防枚举）
var ErrTokenInvalid = errors.New("invalid token")

// IssueToken 签发集群 Token：生成明文（仅本次响应返回）→ 存 SHA256 哈希。
// longterm 轮换语义：现有 Current → Previous 进入宽限期（§6.4）。
func (s *Service) IssueToken(ctx context.Context, clusterID string, req *modules.IssueTokenRequest) (*modules.IssuedToken, *errcode.Response) {
	if req == nil || !modules.TokenKindValid(req.Kind) {
		return nil, errcode.ErrInvalidParam
	}
	if _, rsp := s.getCluster(ctx, clusterID); rsp != nil {
		return nil, rsp
	}

	switch req.Kind {
	case modules.TokenKindLongterm:
		return s.issueLongterm(ctx, clusterID)
	default:
		return s.issueRegister(ctx, clusterID)
	}
}

// issueLongterm 签发/轮换长期 Token（内部复用：签发 API 与注册交换共用）
func (s *Service) issueLongterm(ctx context.Context, clusterID string) (*modules.IssuedToken, *errcode.Response) {
	plain, hash, err := newTokenPlain(modules.TokenKindLongterm)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}

	t := &modules.ClusterToken{ClusterID: clusterID, Kind: string(modules.TokenKindLongterm), CurrentHash: hash}
	issued := &modules.IssuedToken{Token: plain, Kind: modules.TokenKindLongterm}

	// 轮换：旧 Token 降级进宽限期；首次签发则无 Previous
	existing, err := s.d.GetToken(ctx, clusterID, modules.TokenKindLongterm)
	switch {
	case err == nil:
		t.PreviousHash = existing.CurrentHash
		grace := time.Now().Add(tokenGracePeriod)
		t.GraceEndsAt = grace
		issued.GraceEndsAt = &grace
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 首次签发
	default:
		return nil, errcode.ServerInternal(err)
	}

	if err := s.d.UpsertToken(ctx, t); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return issued, nil
}

// issueRegister 签发注册 Token（1h 有效）
func (s *Service) issueRegister(ctx context.Context, clusterID string) (*modules.IssuedToken, *errcode.Response) {
	plain, hash, err := newTokenPlain(modules.TokenKindRegister)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	t := &modules.ClusterToken{
		ClusterID:   clusterID,
		Kind:        string(modules.TokenKindRegister),
		CurrentHash: hash,
		ExpiresAt:   time.Now().Add(registerTokenTTL),
	}
	if err := s.d.UpsertToken(ctx, t); err != nil {
		return nil, errcode.ServerInternal(err)
	}
	return &modules.IssuedToken{Token: plain, Kind: modules.TokenKindRegister}, nil
}

// ExchangeRegisterToken Agent 首次注册：注册 Token 换长期 Token（设计文档 §6.1）。
// 一次性语义：交换成功后立即删除注册 Token；类型不对或已失效一律 ErrTokenInvalid
// （调用方统一 401，防枚举）。
func (s *Service) ExchangeRegisterToken(ctx context.Context, registerToken string) (*modules.IssuedToken, error) {
	clusterID, kind, err := s.LookupToken(ctx, registerToken)
	if err != nil || kind != modules.TokenKindRegister {
		return nil, ErrTokenInvalid
	}

	issued, rsp := s.issueLongterm(ctx, clusterID)
	if rsp != nil {
		return nil, fmt.Errorf("service -> issue longterm on exchange: %s", rsp)
	}
	// 一次性：删除注册 Token（删除失败不回滚签发——注册 Token 已宽限语义外，
	// 下轮轮换/吊销收敛，此处仅记录）
	if _, err := s.d.DeleteToken(ctx, clusterID, modules.TokenKindRegister); err != nil {
		log.Printf("[platform] exchange: delete register token %s: %v", clusterID, err)
	}
	return issued, nil
}

// LookupToken 校验 Token 明文，返回归属集群与类型（模式 A，供 /tunnel 入口使用）。
// 命中规则：当前哈希，或宽限期内且未过期的旧哈希；其余一律 ErrTokenInvalid。
func (s *Service) LookupToken(ctx context.Context, token string) (string, modules.TokenKind, error) {
	if token == "" {
		return "", "", ErrTokenInvalid
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])

	t, inGrace, err := s.d.FindTokenByHash(ctx, hash)
	if err != nil {
		return "", "", fmt.Errorf("service -> lookup token: %w", err)
	}
	if t == nil {
		return "", "", ErrTokenInvalid
	}
	// 当前 Token 过期（注册 Token 1h）即失效；宽限期内的旧 Token 不受 ExpiresAt 约束之外
	// 的额外检查（宽限判定已在 SQL 中完成）
	if !inGrace && !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return "", "", ErrTokenInvalid
	}
	return t.ClusterID, modules.TokenKind(t.Kind), nil
}

// ListTokens 集群 Token 元数据列表（不含哈希与明文）
func (s *Service) ListTokens(ctx context.Context, clusterID string) ([]*modules.TokenDTO, *errcode.Response) {
	if _, rsp := s.getCluster(ctx, clusterID); rsp != nil {
		return nil, rsp
	}
	list, err := s.d.ListTokens(ctx, clusterID)
	if err != nil {
		return nil, errcode.ServerInternal(err)
	}
	dtos := make([]*modules.TokenDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, list[i].ToDTO())
	}
	return dtos, nil
}

// RevokeToken 注销 Token（立即失效，无宽限）
func (s *Service) RevokeToken(ctx context.Context, clusterID string, kind modules.TokenKind) *errcode.Response {
	if !modules.TokenKindValid(kind) {
		return errcode.ErrInvalidParam
	}
	rows, err := s.d.DeleteToken(ctx, clusterID, kind)
	if err != nil {
		return errcode.ServerInternal(err)
	}
	if rows == 0 {
		return errcode.ErrTokenNotFound
	}
	return nil
}

// newTokenPlain 生成 Token 明文与 SHA256 哈希（明文前缀带类型，便于辨识）
func newTokenPlain(kind modules.TokenKind) (plain, hash string, err error) {
	prefix := modules.TokenPrefixLongterm
	if kind == modules.TokenKindRegister {
		prefix = modules.TokenPrefixRegister
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("service -> token rand: %w", err)
	}
	plain = prefix + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}
