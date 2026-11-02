package modules

import "time"

// Robot CICD 机器人身份：绑定到某个集群与角色，使用长期 JWT 的 static kubeconfig。
// JTI/IssuedAt/ExpiresAt 用于再次签发同一 claims 的 kubeconfig，以及删除时吊销 Token。
type Robot struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	Name      string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	ClusterID string    `gorm:"size:64;not null;index" json:"cluster_id"`
	Sub       string    `gorm:"size:128;not null;uniqueIndex" json:"sub"`
	Role      string    `gorm:"size:256;not null;default:''" json:"role"`
	JTI       string    `gorm:"size:64;not null" json:"-"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// RobotDTO 机器人列表项（不含 Token 明文/JTI）。
type RobotDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ClusterID string    `json:"cluster_id"`
	Sub       string    `json:"sub"`
	Role      string    `json:"role"`
	Status    string    `json:"status"` // active / expired
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRobotRequest POST /api/v1/robots
type CreateRobotRequest struct {
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
	Role      string `json:"role"`
}

// ToDTO 转为列表项；status 按到期时间计算。
func (r *Robot) ToDTO(now time.Time) *RobotDTO {
	status := "active"
	if !r.ExpiresAt.After(now) {
		status = "expired"
	}
	return &RobotDTO{
		ID:        r.ID,
		Name:      r.Name,
		ClusterID: r.ClusterID,
		Sub:       r.Sub,
		Role:      r.Role,
		Status:    status,
		IssuedAt:  r.IssuedAt,
		ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt,
	}
}
