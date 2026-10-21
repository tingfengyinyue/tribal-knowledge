package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type Node struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	NodeKey    string    `gorm:"uniqueIndex;not null" json:"node_key"`
	NodeType   string    `gorm:"not null" json:"node_type"`
	Name       string    `json:"name"`
	Attributes JSONB     `gorm:"type:jsonb;default:'{}'" json:"attributes"`
	Status     string    `gorm:"default:active" json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Node) TableName() string { return "tk_nodes" }

type Edge struct {
	ID           int64     `gorm:"primaryKey" json:"id"`
	SourceNodeID int64     `gorm:"not null" json:"source_node_id"`
	TargetNodeID int64     `gorm:"not null" json:"target_node_id"`
	EdgeType     string    `gorm:"not null" json:"edge_type"`
	Attributes   JSONB     `gorm:"type:jsonb;default:'{}'" json:"attributes"`
	Status       string    `gorm:"default:active" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Edge) TableName() string { return "tk_edges" }

// JSONB wraps map[string]any for gorm jsonb columns.
type JSONB map[string]any

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

func (j *JSONB) Scan(src any) error {
	if src == nil {
		*j = JSONB{}
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	}
	return nil
}
