package model

import "time"

type Session struct {
	ID               int64     `gorm:"primaryKey" json:"id"`
	UserID           int64     `gorm:"default:0" json:"user_id"`
	Question         string    `json:"question"`
	QuestionAbstract string    `json:"question_abstract"`
	ClientName       string    `json:"client_name"`
	BootstrapVersion string    `json:"bootstrap_version"`
	GitRemote        string    `json:"git_remote"`
	Parsed           JSONB     `gorm:"type:jsonb;default:'{}'" json:"parsed"`
	Conclusion       JSONB     `gorm:"type:jsonb" json:"conclusion"`
	Status           string    `gorm:"default:open" json:"status"` // open, closed
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Session) TableName() string { return "tk_sessions" }

type Step struct {
	ID          int64      `gorm:"primaryKey" json:"id"`
	SessionID   int64      `gorm:"not null" json:"session_id"`
	Seq         int        `gorm:"not null" json:"seq"`
	StepType    string     `gorm:"not null" json:"step_type"` // env_check, plan, query, note
	Tool        string     `json:"tool"`
	Command     string     `json:"command"`
	ResourceKey string     `json:"resource_key"`
	ResultClass string     `json:"result_class"` // ok, no_data, wrong_mapping, ...
	RowsCount   int        `json:"rows_count"`
	Summary     string     `json:"summary"`
	IDsFound    JSONB      `gorm:"type:jsonb;default:'{}'" json:"ids_found"`
	UsedItemIDs Int64Array `gorm:"type:bigint[]" json:"used_item_ids"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (Step) TableName() string { return "tk_steps" }

type Update struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	SessionID  int64     `gorm:"not null" json:"session_id"`
	UpdateID   string    `json:"update_id"`
	Feedback   JSONB     `gorm:"type:jsonb;default:'[]'" json:"feedback"`
	Reflection JSONB     `gorm:"type:jsonb;default:'{}'" json:"reflection"`
	Operations JSONB     `gorm:"type:jsonb;default:'[]'" json:"operations"`
	Results    JSONB     `gorm:"type:jsonb;default:'[]'" json:"results"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Update) TableName() string { return "tk_updates" }

type Case struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	SessionID     int64     `gorm:"not null" json:"session_id"`
	ItemID        *int64    `json:"item_id"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	StepsSnapshot JSONB     `gorm:"type:jsonb;default:'[]'" json:"steps_snapshot"`
	Conclusion    JSONB     `gorm:"type:jsonb;default:'{}'" json:"conclusion"`
	CreatedAt     time.Time `json:"created_at"`
}

func (Case) TableName() string { return "tk_cases" }
