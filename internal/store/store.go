package store

import (
	"context"
	"fmt"
	"time"

	"github.com/tingfengyinyue/tribal-knowledge/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func New(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

// ---- Nodes ----

func (s *Store) GetNode(ctx context.Context, nodeKey string) (*model.Node, error) {
	var n model.Node
	if err := s.db.WithContext(ctx).Where("node_key = ? AND status = 'active'", nodeKey).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) GetNodeByID(ctx context.Context, id int64) (*model.Node, error) {
	var n model.Node
	if err := s.db.WithContext(ctx).First(&n, id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) UpsertNode(ctx context.Context, n *model.Node) error {
	return s.db.WithContext(ctx).
		Where("node_key = ?", n.NodeKey).
		Assign(model.Node{
			NodeType:   n.NodeType,
			Name:       n.Name,
			Attributes: n.Attributes,
			Status:     "active",
		}).
		FirstOrCreate(n).Error
}

func (s *Store) UpdateNode(ctx context.Context, n *model.Node) error {
	return s.db.WithContext(ctx).Save(n).Error
}

func (s *Store) GetServiceByRepo(ctx context.Context, repo string) (*model.Node, error) {
	var n model.Node
	err := s.db.WithContext(ctx).
		Where("node_type = 'service' AND status = 'active' AND lower(attributes->>'repo') = lower(?)", repo).
		First(&n).Error
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) ListActiveServices(ctx context.Context) ([]model.Node, error) {
	var nodes []model.Node
	err := s.db.WithContext(ctx).
		Where("node_type = 'service' AND status = 'active'").
		Find(&nodes).Error
	return nodes, err
}

// ---- Edges ----

func (s *Store) UpsertEdge(ctx context.Context, e *model.Edge) error {
	return s.db.WithContext(ctx).
		Where("source_node_id = ? AND target_node_id = ? AND edge_type = ?", e.SourceNodeID, e.TargetNodeID, e.EdgeType).
		Assign(model.Edge{Attributes: e.Attributes, Status: "active"}).
		FirstOrCreate(e).Error
}

func (s *Store) InvalidateEdge(ctx context.Context, sourceID, targetID int64, edgeType string) (int64, error) {
	res := s.db.WithContext(ctx).
		Model(&model.Edge{}).
		Where("source_node_id = ? AND target_node_id = ? AND edge_type = ? AND status = 'active'", sourceID, targetID, edgeType).
		Updates(map[string]any{"status": "invalid", "updated_at": time.Now()})
	return res.RowsAffected, res.Error
}

func (s *Store) GetNeighbors(ctx context.Context, nodeID int64) ([]model.Edge, error) {
	var edges []model.Edge
	err := s.db.WithContext(ctx).
		Where("(source_node_id = ? OR target_node_id = ?) AND status = 'active'", nodeID, nodeID).
		Find(&edges).Error
	return edges, err
}

// ---- Items ----

func (s *Store) CreateItem(ctx context.Context, it *model.Item) error {
	return s.db.WithContext(ctx).Create(it).Error
}

func (s *Store) GetItem(ctx context.Context, id int64) (*model.Item, error) {
	var it model.Item
	if err := s.db.WithContext(ctx).First(&it, id).Error; err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Store) UpdateItem(ctx context.Context, it *model.Item) error {
	return s.db.WithContext(ctx).Save(it).Error
}

func (s *Store) ListItemsByType(ctx context.Context, itemType string) ([]model.Item, error) {
	var items []model.Item
	err := s.db.WithContext(ctx).
		Where("item_type = ? AND status = 'active'", itemType).
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	return items, err
}

func (s *Store) ListAlwaysLoadItems(ctx context.Context) ([]model.Item, error) {
	var items []model.Item
	err := s.db.WithContext(ctx).
		Where("load_when = 'always' AND status = 'active'").
		Order("sort_order ASC, id ASC").
		Find(&items).Error
	return items, err
}

func (s *Store) TouchVerified(ctx context.Context, ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	ts := at.Format(time.RFC3339)
	return s.db.WithContext(ctx).
		Model(&model.Item{}).
		Where("id IN ? AND status = 'active' AND item_type IN ?", ids, model.EmbeddedItemTypes).
		UpdateColumn("attributes",
			gorm.Expr("attributes || jsonb_build_object('last_verified_at', ?::text)", ts),
		).Error
}

// ---- Sessions ----

func (s *Store) CreateSession(ctx context.Context, sess *model.Session) error {
	return s.db.WithContext(ctx).Create(sess).Error
}

func (s *Store) GetSession(ctx context.Context, id int64) (*model.Session, error) {
	var sess model.Session
	if err := s.db.WithContext(ctx).First(&sess, id).Error; err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess *model.Session) error {
	return s.db.WithContext(ctx).Save(sess).Error
}

// ---- Steps ----

func (s *Store) UpsertStep(ctx context.Context, step *model.Step) error {
	return s.db.WithContext(ctx).
		Where("session_id = ? AND seq = ?", step.SessionID, step.Seq).
		Assign(*step).
		FirstOrCreate(step).Error
}

func (s *Store) ListSteps(ctx context.Context, sessionID int64) ([]model.Step, error) {
	var steps []model.Step
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq ASC").
		Find(&steps).Error
	return steps, err
}

// ---- Updates ----

func (s *Store) CreateUpdate(ctx context.Context, u *model.Update) error {
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *Store) GetUpdateByKey(ctx context.Context, sessionID int64, updateID string) (*model.Update, error) {
	if updateID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var u model.Update
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND update_id = ?", sessionID, updateID).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ---- Cases ----

func (s *Store) CreateCase(ctx context.Context, c *model.Case) error {
	return s.db.WithContext(ctx).Create(c).Error
}

// ---- Item History ----

func (s *Store) CreateItemHistory(ctx context.Context, h *model.ItemHistory) error {
	return s.db.WithContext(ctx).Create(h).Error
}

func (s *Store) ListHistoryBySession(ctx context.Context, sessionID int64) ([]model.ItemHistory, error) {
	var history []model.ItemHistory
	err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id ASC").
		Find(&history).Error
	return history, err
}

// ---- Graph expansion ----

func (s *Store) ExpandGraph(ctx context.Context, nodeIDs []int64) (services []model.Node, resources []model.Node, neighbors []model.Edge, err error) {
	if len(nodeIDs) == 0 {
		return
	}
	err = s.db.WithContext(ctx).
		Where("id IN ? AND status = 'active' AND node_type = 'service'", nodeIDs).
		Find(&services).Error
	if err != nil {
		return
	}
	err = s.db.WithContext(ctx).
		Where("id IN ? AND status = 'active' AND node_type = 'resource'", nodeIDs).
		Find(&resources).Error
	if err != nil {
		return
	}
	err = s.db.WithContext(ctx).
		Where("(source_node_id IN ? OR target_node_id IN ?) AND status = 'active'", nodeIDs, nodeIDs).
		Find(&neighbors).Error
	return
}

func (s *Store) Begin() *gorm.DB {
	return s.db.Begin()
}

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
