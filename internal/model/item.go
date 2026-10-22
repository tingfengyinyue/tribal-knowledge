package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

type Item struct {
	ID         int64            `gorm:"primaryKey" json:"id"`
	ItemType   string           `gorm:"not null" json:"item_type"` // experience, document, case, instruction
	Title      string           `json:"title"`
	Summary    string           `json:"summary"`
	Content    string           `json:"content"`
	Tags       StringArray      `gorm:"type:text[]" json:"tags"`
	NodeIDs    Int64Array       `gorm:"type:bigint[]" json:"node_ids"`
	Attributes JSONB            `gorm:"type:jsonb;default:'{}'" json:"attributes"`
	Trust      float32          `gorm:"default:1.0" json:"trust"`
	Status     string           `gorm:"default:active" json:"status"` // active, invalid, superseded
	LoadWhen   string           `gorm:"default:relevant" json:"load_when"`
	SortOrder  int              `gorm:"default:0" json:"sort_order"`
	Embedding  *pgvector.Vector `gorm:"type:vector" json:"-"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

func (Item) TableName() string { return "tk_items" }

type ItemHistory struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	ItemID     int64     `gorm:"not null" json:"item_id"`
	SessionID  int64     `gorm:"not null" json:"session_id"`
	Op         string    `gorm:"not null" json:"op"` // created, updated, invalidated
	BeforeData JSONB     `gorm:"type:jsonb" json:"before_data"`
	AfterData  JSONB     `gorm:"type:jsonb;not null" json:"after_data"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ItemHistory) TableName() string { return "tk_item_history" }

// EmbeddedItemTypes are the item types whose embeddings are computed and stored.
var EmbeddedItemTypes = []string{"experience", "document", "case"}
