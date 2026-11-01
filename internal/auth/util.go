package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
)

// randomID 生成 jti（128bit 随机 hex）
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 随机源失败属系统性故障，无法安全降级
		panic(fmt.Sprintf("auth: rand: %v", err))
	}
	return hex.EncodeToString(b)
}

// base64URL JWT 标准的 URL-safe 无填充 base64
func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// bigEndianBytes 大端序编码（RSA 公钥指数 e 通常为 65537 → "AQAB"）
func bigEndianBytes(n int) []byte {
	return new(big.Int).SetInt64(int64(n)).Bytes()
}

// jsonMarshal 统一 JSON 编码入口（预留缩进/顺序控制位）
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
