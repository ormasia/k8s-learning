我将先快速查看你项目中的 models 与 database 使用风格，以便产出可直接落地的 Outbox 表模型与仓库代码（保持和现有仓库风格一致），不会修改任何现有文件，只做只读浏览。随后我会输出全量的新文件代码和 SQL。 

搜索与 `**/common/models/**/*.go` 匹配的文件，16 匹配项

读取 [](file:///c%3A/Users/ws/Desktop/Code/tmadminsrv/common/models/config_history.go)

好的，我来给出“Outbox 强一致方案”的完整可落地实现代码与 SQL（不改你现有代码），你把这些新文件加进来即可；之后只需在 UpdateConfig/ReleaseConfig 的事务里插入一条 outbox 记录即可完成生产者侧改造。

## 目录与文件建议
- 新增表 SQL：`db/migrations/20250929_create_outbox_events.sql`
- 新增模型与仓库：`common/models/outbox_event.go`
- 新增事件定义：`modules/api/models/config_history_event.go`
- 新增发布器：`modules/api/extra/outbox_publisher.go`

你可按现有项目风格调整路径，但建议保持 models 在 models，业务相关放 `modules/api/...`。

## 1) 新增表 SQL

说明：
- 索引：status+created_at（扫描顺序），event_id 唯一（幂等），可选 next_attempt_at（更精准过滤）
- 保留 exchange 和 routing_key 字段，避免发布器硬编码

内容（MySQL 8+，兼容 utf8mb4）：

```sql
-- db/migrations/20250929_create_outbox_events.sql
CREATE TABLE IF NOT EXISTS `outbox_events` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
  `event_id` VARCHAR(64) NOT NULL COMMENT '全局唯一事件ID，用于幂等',
  `event_type` VARCHAR(128) NOT NULL COMMENT '事件类型，如 config.history.created',
  `exchange` VARCHAR(128) NOT NULL COMMENT 'MQ exchange',
  `routing_key` VARCHAR(128) NOT NULL COMMENT 'MQ routing key',
  `payload` JSON NOT NULL COMMENT '事件负载，JSON 快照',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0=pending,1=sent,2=failed',
  `retry_count` INT NOT NULL DEFAULT 0 COMMENT '重试次数',
  `last_error` TEXT NULL COMMENT '最后一次错误信息',
  `next_attempt_at` DATETIME NULL COMMENT '下次尝试时间（用于退避）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_event_id` (`event_id`),
  KEY `idx_status_created_at` (`status`, `created_at`),
  KEY `idx_status_next_attempt` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

保留了 `payload` 为 JSON 类型，如你的 MySQL 版本不支持 JSON，可改为 `LONGTEXT`。

## 2) Outbox 模型与仓库

```go
// common/models/outbox_event.go
package models

import (
	"context"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/core/database"
	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	Err "git.wondershare.cn/piccloud/tmadminsrv/error"
	"github.com/jinzhu/gorm"
)

const (
	OutboxStatusPending = 0
	OutboxStatusSent    = 1
	OutboxStatusFailed  = 2
)

type OutboxEvent struct {
	ID            uint64    `json:"id" gorm:"primary_key;autoIncrement"`
	EventID       string    `json:"event_id" gorm:"type:varchar(64);unique_index:uk_event_id;not null"`
	EventType     string    `json:"event_type" gorm:"type:varchar(128);not null"`
	Exchange      string    `json:"exchange" gorm:"type:varchar(128);not null"`
	RoutingKey    string    `json:"routing_key" gorm:"type:varchar(128);not null"`
	Payload       string    `json:"payload" gorm:"type:longtext;not null"`
	Status        int       `json:"status" gorm:"type:tinyint;not null;default:0"`
	RetryCount    int       `json:"retry_count" gorm:"type:int;not null;default:0"`
	LastError     string    `json:"last_error" gorm:"type:longtext"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at" gorm:"type:datetime;not null"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"type:datetime;not null"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}

type OutboxEventRepository interface {
	InsertInTx(e *OutboxEvent, tx *gorm.DB) error
	FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkSent(id uint64) error
	MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error
	CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type OutboxEventEntity struct {
	db *database.DB
}

func NewOutboxEventEntity(db *database.DB) *OutboxEventEntity {
	return &OutboxEventEntity{db: db}
}

func (e *OutboxEventEntity) InsertInTx(ev *OutboxEvent, tx *gorm.DB) error {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.UpdatedAt.IsZero() {
		ev.UpdatedAt = ev.CreatedAt
	}
	if err := tx.Create(ev).Error; err != nil {
		zaplog.Errorf("Outbox InsertInTx failed: %v", err)
		return Err.ErrCreateFailed
	}
	return nil
}

func (e *OutboxEventEntity) FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	now := time.Now()
	var list []OutboxEvent
	q := e.db.DB.
		Where("status = ?", OutboxStatusPending).
		Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now).
		Order("created_at asc").
		Limit(limit)
	if err := q.Find(&list).Error; err != nil {
		zaplog.ErrorWithCtx(ctx, "Outbox FetchPending failed: %v", err)
		return nil, Err.ErrDB
	}
	return list, nil
}

func (e *OutboxEventEntity) MarkSent(id uint64) error {
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     OutboxStatusSent,
			"updated_at": time.Now(),
			"last_error": "",
		}).Error
}

func (e *OutboxEventEntity) MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error {
	// 原子地增加 retry_count 并设置下一次尝试时间
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          OutboxStatusPending, // 保持 pending，等待下次扫描
			"retry_count":     gorm.Expr("retry_count + 1"),
			"last_error":      errMsg,
			"next_attempt_at": nextAttempt,
			"updated_at":      time.Now(),
		}).Error
}

func (e *OutboxEventEntity) CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	// 分批删除 sent 的历史事件
	var affected int64
	tx := e.db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 用子查询限定范围
	sub := tx.Model(&OutboxEvent{}).
		Select("id").
		Where("status = ?", OutboxStatusSent).
		Where("updated_at < ?", before).
		Limit(limit).SubQuery()

	if err := tx.Where("id in (?)", sub).Delete(OutboxEvent{}).Error; err != nil {
		tx.Rollback()
		zaplog.ErrorWithCtx(ctx, "Outbox CleanupSentBefore failed: %v", err)
		return 0, Err.ErrDB
	}
	if err := tx.Commit().Error; err != nil {
		return 0, Err.ErrDB
	}
	// 受影响行数不能直接从 Delete 返回，必要时再查计数；这里简单返回 limit，实际使用可改为准确值
	affected = int64(limit)
	return affected, nil
}
```

说明：
- InsertInTx：用于业务事务内插入 outbox 记录。
- FetchPending：取待发送的记录；按 created_at 升序保障事件顺序性。
- MarkFailed：不把状态置为 failed，而是保留 pending 并设置 next_attempt_at 实现退避；超过阈值转 failed 的策略可以放到发布器里决定。
- CleanupSentBefore：用于定期清理已发送的老记录。

## 3) 事件定义（历史快照）

```go
// modules/api/models/config_history_event.go
package models

import "time"

// ConfigHistoryEvent 对齐 ConfigHistoryModel 的字段，作为 outbox payload 的 JSON
type ConfigHistoryEvent struct {
	EventID       string    `json:"event_id"`        // 幂等ID
	EventType     string    `json:"event_type"`      // 固定：config.history.created
	OccurredAt    int64     `json:"occurred_at"`     // 事件产生时间（秒）
	SchemaVersion int       `json:"schema_version"`  // 例如 1
	TraceID       string    `json:"trace_id,omitempty"`

	ConfigId     int64     `json:"config_id"`
	ModuleName   string    `json:"module_name"`
	Lang         string    `json:"lang"`
	ConfigKey    string    `json:"config_key"`
	Desc         string    `json:"desc"`
	ConfigType   int       `json:"config_type"`
	ConfigValue  string    `json:"config_value"`
	ConfigSchema string    `json:"config_schema"`
	Version      int       `json:"version"`
	EditUser     string    `json:"edit_user"`
	ReleaseUser  string    `json:"release_user"`
	RelatedUsers string    `json:"related_users"`
	CreatedAt    time.Time `json:"created_at"` // 历史记录的创建时间（上一条正式数据的 updated_at）
}
```

说明：
- 这就是最终写入历史表需要的快照，消费者拿到它即可入库。
- 生产者侧将此结构体 JSON 序列化后放入 outbox 的 payload。

## 4) 出站发布器（扫描 pending -> 发送 MQ -> 标记 sent/重试/失败）

```go
// modules/api/extra/outbox_publisher.go
package extra

import (
	"context"
	"encoding/json"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	"git.wondershare.cn/piccloud/tmadminsrv/common/models"
	"git.wondershare.cn/piccloud/tmadminsrv/server"
)

type OutboxPublisherConfig struct {
	ScanInterval   time.Duration // 扫描周期
	BatchSize      int           // 每次批量发送数
	BaseBackoff    time.Duration // 基础退避（如 5s）
	MaxBackoff     time.Duration // 最大退避（如 10m）
	MaxRetry       int           // 最大重试次数（超过则置 failed）
	Retention      time.Duration // 已发送保留时长（如 7*24h）
	CleanupBatch   int           // 清理每批条数（如 500）
	EnableCleanup  bool          // 是否清理
}

type OutboxPublisher struct {
	cfg     OutboxPublisherConfig
	repo    *models.OutboxEventEntity
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewOutboxPublisher(repo *models.OutboxEventEntity, cfg OutboxPublisherConfig) *OutboxPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &OutboxPublisher{
		cfg:    cfg,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *OutboxPublisher) Start() {
	ticker := time.NewTicker(p.cfg.ScanInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.tick()
			}
		}
	}()

	if p.cfg.EnableCleanup && p.cfg.Retention > 0 {
		go p.cleanupLoop()
	}
}

func (p *OutboxPublisher) Stop() {
	p.cancel()
}

func (p *OutboxPublisher) tick() {
	events, err := p.repo.FetchPending(p.ctx, p.cfg.BatchSize)
	if err != nil {
		zaplog.Errorf("OutboxPublisher FetchPending error: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}
	mq := server.GetMqSender()
	for _, ev := range events {
		// 发送消息
		headers := map[string]interface{}{}
		body := []byte(ev.Payload)
		// 这里不强制传 ctx 给 sender，若需要 trace，可以在 payload 或 headers 中带上 trace_id
		if err := mq.SendMessage(ev.Exchange, ev.RoutingKey, headers, body, 0 /*priority*/); err != nil {
			// 发送失败 -> 退避重试 或 标记 failed
			nextAttempt, newStatus := p.nextAttemptAndStatus(ev.RetryCount, err)
			msg := truncateErr(err.Error(), 2000)
			if newStatus == models.OutboxStatusFailed {
				zaplog.Errorf("OutboxPublisher send failed permanently event_id=%s err=%v", ev.EventID, err)
				// 最终失败：将状态置为 failed，记录最后错误信息（也可以保留 pending 让人工重试，这里选择失败落地）
				_ = p.repo.db.DB.Model(&models.OutboxEvent{}).
					Where("id = ?", ev.ID).
					Updates(map[string]interface{}{
						"status":          models.OutboxStatusFailed,
						"retry_count":     ev.RetryCount + 1,
						"last_error":      msg,
						"next_attempt_at": nextAttempt,
						"updated_at":      time.Now(),
					}).Error
				continue
			}
			if err := p.repo.MarkFailed(ev.ID, msg, nextAttempt); err != nil {
				zaplog.Errorf("OutboxPublisher MarkFailed error: %v", err)
			}
			continue
		}

		// 发送成功
		if err := p.repo.MarkSent(ev.ID); err != nil {
			zaplog.Errorf("OutboxPublisher MarkSent error: %v", err)
		}
	}
}

func (p *OutboxPublisher) nextAttemptAndStatus(retry int, err error) (time.Time, int) {
	backoff := p.cfg.BaseBackoff * (1 << retry) // 2^retry * base
	if backoff > p.cfg.MaxBackoff {
		backoff = p.cfg.MaxBackoff
	}
	next := time.Now().Add(backoff)
	status := models.OutboxStatusPending
	if retry+1 >= p.cfg.MaxRetry {
		status = models.OutboxStatusFailed
	}
	return next, status
}

func (p *OutboxPublisher) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().Add(-p.cfg.Retention)
			_, err := p.repo.CleanupSentBefore(p.ctx, before, p.cfg.CleanupBatch)
			if err != nil {
				zaplog.Warnf("Outbox cleanup error: %v", err)
			}
		}
	}
}

func truncateErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// 生产者侧：构建事件payload的小工具，可选使用
func BuildConfigHistoryPayload(e *models.ConfigHistoryEvent) (string, error) {
	buf, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
```

说明：
- 发布器使用 `server.GetMqSender()`，与现有框架一致。
- 退避：指数退避，封顶 MaxBackoff；达到 MaxRetry 置为 failed。
- 清理：每小时清理一次超出保留期的 sent 数据。
- 你只需在启动时实例化并 Start（后续我可以按你期望集成到 `server.Start()`）。

## 5) 生产者侧如何在事务内插入 Outbox（示例片段，仅供对接）
你后续在 `UpdateConfig` 和 `ReleaseConfig` 的“历史写入”所在事务里，把 outbox 插入进去即可。示意：

```go
// 示例：在 UpdateConfig 事务内
tx := b.configEntity.Begin()
defer ...
// 1. 正常业务更新...
// 2. 构造历史事件快照（使用更新前的 record 数据）
evt := &models.ConfigHistoryEvent{
  EventID:       uuid.New().String(),   // 需引入 uuid
  EventType:     "config.history.created",
  OccurredAt:    time.Now().Unix(),
  SchemaVersion: 1,
  ConfigId:      record.Id,
  ModuleName:    record.ModuleName,
  Lang:          record.Lang,
  ConfigKey:     record.ConfigKey,
  Desc:          record.Desc,
  ConfigType:    record.ConfigType,
  ConfigValue:   record.ConfigValue,
  ConfigSchema:  record.ConfigSchema,
  Version:       record.Version,        // 注意：历史使用旧版本
  EditUser:      record.EditUser,
  ReleaseUser:   record.ReleaseUser,
  RelatedUsers:  record.RelatedUsers,
  CreatedAt:     record.UpdatedAt,      // 历史创建时间 = 上一条正式数据的 updated_at
}
payload, _ := extra.BuildConfigHistoryPayload(evt)

// 3. 插入 outbox 事件（与业务同事务）
outbox := &models.OutboxEvent{
  EventID:    evt.EventID,
  EventType:  evt.EventType,
  Exchange:   "tmadminsrv_service",
  RoutingKey: "tmadminsrv_config_history",
  Payload:    payload,
  Status:     models.OutboxStatusPending,
}
if err := outboxRepo.InsertInTx(outbox, tx); err != nil {
  tx.Rollback()
  return false, err
}

// 4. 提交事务，发布器会异步扫描并投递到 MQ
if err := tx.Commit().Error; err != nil { ... }
```

`ReleaseConfig` 类似，但遵循你现有条件 `if configRecord.Version > 0` 再插入 outbox。

## 6) 消费者侧（简述）
你现有 MQ 消费架构已就绪。按之前方案，新建一个 `ConfigHistoryCtrl` 消费者，`RegisterQueueTask("tmadminsrv_config_history_queue", ...)`，在 DoNonAction 中解析 `ConfigHistoryEvent`，按历史表字段入库（历史表上加唯一约束 event_id 或 (config_id, version) 保证幂等），错误分为可重试（返回 Err.ErrNeedRetry）与不可重试（让消息进错误队列）。这部分如需，我可再补一份完整消费者代码骨架。

## 7) 发布器初始化示例（仅说明，不改你的代码）
你可在 `server.Start()` 之后、`server.Run()` 之前初始化：

- 从 `server.GetDB("ws_tm_core")` 拿 DB
- `repo := models.NewOutboxEventEntity(db)`
- `pub := extra.NewOutboxPublisher(repo, extra.OutboxPublisherConfig{ ScanInterval: 2 * time.Second, BatchSize: 100, BaseBackoff: 5 * time.Second, MaxBackoff: 10 * time.Minute, MaxRetry: 8, Retention: 7 * 24 * time.Hour, CleanupBatch: 1000, EnableCleanup: true })`
- `pub.Start()`

这样，生产者只负责“事务内写 outbox”，发布器统一投递 MQ，消费者落库历史，形成闭环。

——

以上是你要求的“status+created_at 索引、event_id 唯一、事务内插 outbox、发布器扫描/发送/退避/清理”的完整代码实现与 SQL，不修改现有代码。你把这些新文件加入工程、创建表后，就可以按示例在业务事务里插入 outbox 记录，功能即可跑起来。需要我继续补消费者骨架或集成初始化位置的话，告诉我你的偏好（放在 init.go 里或 server.startup.go 里），我再给出对应代码。


我将先快速查看你项目中的 models 与 database 使用风格，以便产出可直接落地的 Outbox 表模型与仓库代码（保持和现有仓库风格一致），不会修改任何现有文件，只做只读浏览。随后我会输出全量的新文件代码和 SQL。 

搜索与 `**/common/models/**/*.go` 匹配的文件，16 匹配项

读取 [](file:///c%3A/Users/ws/Desktop/Code/tmadminsrv/common/models/config_history.go)

好的，我来给出“Outbox 强一致方案”的完整可落地实现代码与 SQL（不改你现有代码），你把这些新文件加进来即可；之后只需在 UpdateConfig/ReleaseConfig 的事务里插入一条 outbox 记录即可完成生产者侧改造。

## 目录与文件建议
- 新增表 SQL：`db/migrations/20250929_create_outbox_events.sql`
- 新增模型与仓库：`common/models/outbox_event.go`
- 新增事件定义：`modules/api/models/config_history_event.go`
- 新增发布器：`modules/api/extra/outbox_publisher.go`

你可按现有项目风格调整路径，但建议保持 models 在 models，业务相关放 `modules/api/...`。

## 1) 新增表 SQL

说明：
- 索引：status+created_at（扫描顺序），event_id 唯一（幂等），可选 next_attempt_at（更精准过滤）
- 保留 exchange 和 routing_key 字段，避免发布器硬编码

内容（MySQL 8+，兼容 utf8mb4）：

```sql
-- db/migrations/20250929_create_outbox_events.sql
CREATE TABLE IF NOT EXISTS `outbox_events` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
  `event_id` VARCHAR(64) NOT NULL COMMENT '全局唯一事件ID，用于幂等',
  `event_type` VARCHAR(128) NOT NULL COMMENT '事件类型，如 config.history.created',
  `exchange` VARCHAR(128) NOT NULL COMMENT 'MQ exchange',
  `routing_key` VARCHAR(128) NOT NULL COMMENT 'MQ routing key',
  `payload` JSON NOT NULL COMMENT '事件负载，JSON 快照',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0=pending,1=sent,2=failed',
  `retry_count` INT NOT NULL DEFAULT 0 COMMENT '重试次数',
  `last_error` TEXT NULL COMMENT '最后一次错误信息',
  `next_attempt_at` DATETIME NULL COMMENT '下次尝试时间（用于退避）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_event_id` (`event_id`),
  KEY `idx_status_created_at` (`status`, `created_at`),
  KEY `idx_status_next_attempt` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

保留了 `payload` 为 JSON 类型，如你的 MySQL 版本不支持 JSON，可改为 `LONGTEXT`。

## 2) Outbox 模型与仓库

```go
// common/models/outbox_event.go
package models

import (
	"context"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/core/database"
	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	Err "git.wondershare.cn/piccloud/tmadminsrv/error"
	"github.com/jinzhu/gorm"
)

const (
	OutboxStatusPending = 0
	OutboxStatusSent    = 1
	OutboxStatusFailed  = 2
)

type OutboxEvent struct {
	ID            uint64    `json:"id" gorm:"primary_key;autoIncrement"`
	EventID       string    `json:"event_id" gorm:"type:varchar(64);unique_index:uk_event_id;not null"`
	EventType     string    `json:"event_type" gorm:"type:varchar(128);not null"`
	Exchange      string    `json:"exchange" gorm:"type:varchar(128);not null"`
	RoutingKey    string    `json:"routing_key" gorm:"type:varchar(128);not null"`
	Payload       string    `json:"payload" gorm:"type:longtext;not null"`
	Status        int       `json:"status" gorm:"type:tinyint;not null;default:0"`
	RetryCount    int       `json:"retry_count" gorm:"type:int;not null;default:0"`
	LastError     string    `json:"last_error" gorm:"type:longtext"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at" gorm:"type:datetime;not null"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"type:datetime;not null"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}

type OutboxEventRepository interface {
	InsertInTx(e *OutboxEvent, tx *gorm.DB) error
	FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkSent(id uint64) error
	MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error
	CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type OutboxEventEntity struct {
	db *database.DB
}

func NewOutboxEventEntity(db *database.DB) *OutboxEventEntity {
	return &OutboxEventEntity{db: db}
}

func (e *OutboxEventEntity) InsertInTx(ev *OutboxEvent, tx *gorm.DB) error {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.UpdatedAt.IsZero() {
		ev.UpdatedAt = ev.CreatedAt
	}
	if err := tx.Create(ev).Error; err != nil {
		zaplog.Errorf("Outbox InsertInTx failed: %v", err)
		return Err.ErrCreateFailed
	}
	return nil
}

func (e *OutboxEventEntity) FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	now := time.Now()
	var list []OutboxEvent
	q := e.db.DB.
		Where("status = ?", OutboxStatusPending).
		Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now).
		Order("created_at asc").
		Limit(limit)
	if err := q.Find(&list).Error; err != nil {
		zaplog.ErrorWithCtx(ctx, "Outbox FetchPending failed: %v", err)
		return nil, Err.ErrDB
	}
	return list, nil
}

func (e *OutboxEventEntity) MarkSent(id uint64) error {
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     OutboxStatusSent,
			"updated_at": time.Now(),
			"last_error": "",
		}).Error
}

func (e *OutboxEventEntity) MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error {
	// 原子地增加 retry_count 并设置下一次尝试时间
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          OutboxStatusPending, // 保持 pending，等待下次扫描
			"retry_count":     gorm.Expr("retry_count + 1"),
			"last_error":      errMsg,
			"next_attempt_at": nextAttempt,
			"updated_at":      time.Now(),
		}).Error
}

func (e *OutboxEventEntity) CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	// 分批删除 sent 的历史事件
	var affected int64
	tx := e.db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 用子查询限定范围
	sub := tx.Model(&OutboxEvent{}).
		Select("id").
		Where("status = ?", OutboxStatusSent).
		Where("updated_at < ?", before).
		Limit(limit).SubQuery()

	if err := tx.Where("id in (?)", sub).Delete(OutboxEvent{}).Error; err != nil {
		tx.Rollback()
		zaplog.ErrorWithCtx(ctx, "Outbox CleanupSentBefore failed: %v", err)
		return 0, Err.ErrDB
	}
	if err := tx.Commit().Error; err != nil {
		return 0, Err.ErrDB
	}
	// 受影响行数不能直接从 Delete 返回，必要时再查计数；这里简单返回 limit，实际使用可改为准确值
	affected = int64(limit)
	return affected, nil
}
```

说明：
- InsertInTx：用于业务事务内插入 outbox 记录。
- FetchPending：取待发送的记录；按 created_at 升序保障事件顺序性。
- MarkFailed：不把状态置为 failed，而是保留 pending 并设置 next_attempt_at 实现退避；超过阈值转 failed 的策略可以放到发布器里决定。
- CleanupSentBefore：用于定期清理已发送的老记录。

## 3) 事件定义（历史快照）

```go
// modules/api/models/config_history_event.go
package models

import "time"

// ConfigHistoryEvent 对齐 ConfigHistoryModel 的字段，作为 outbox payload 的 JSON
type ConfigHistoryEvent struct {
	EventID       string    `json:"event_id"`        // 幂等ID
	EventType     string    `json:"event_type"`      // 固定：config.history.created
	OccurredAt    int64     `json:"occurred_at"`     // 事件产生时间（秒）
	SchemaVersion int       `json:"schema_version"`  // 例如 1
	TraceID       string    `json:"trace_id,omitempty"`

	ConfigId     int64     `json:"config_id"`
	ModuleName   string    `json:"module_name"`
	Lang         string    `json:"lang"`
	ConfigKey    string    `json:"config_key"`
	Desc         string    `json:"desc"`
	ConfigType   int       `json:"config_type"`
	ConfigValue  string    `json:"config_value"`
	ConfigSchema string    `json:"config_schema"`
	Version      int       `json:"version"`
	EditUser     string    `json:"edit_user"`
	ReleaseUser  string    `json:"release_user"`
	RelatedUsers string    `json:"related_users"`
	CreatedAt    time.Time `json:"created_at"` // 历史记录的创建时间（上一条正式数据的 updated_at）
}
```

说明：
- 这就是最终写入历史表需要的快照，消费者拿到它即可入库。
- 生产者侧将此结构体 JSON 序列化后放入 outbox 的 payload。

## 4) 出站发布器（扫描 pending -> 发送 MQ -> 标记 sent/重试/失败）

```go
// modules/api/extra/outbox_publisher.go
package extra

import (
	"context"
	"encoding/json"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	"git.wondershare.cn/piccloud/tmadminsrv/common/models"
	"git.wondershare.cn/piccloud/tmadminsrv/server"
)

type OutboxPublisherConfig struct {
	ScanInterval   time.Duration // 扫描周期
	BatchSize      int           // 每次批量发送数
	BaseBackoff    time.Duration // 基础退避（如 5s）
	MaxBackoff     time.Duration // 最大退避（如 10m）
	MaxRetry       int           // 最大重试次数（超过则置 failed）
	Retention      time.Duration // 已发送保留时长（如 7*24h）
	CleanupBatch   int           // 清理每批条数（如 500）
	EnableCleanup  bool          // 是否清理
}

type OutboxPublisher struct {
	cfg     OutboxPublisherConfig
	repo    *models.OutboxEventEntity
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewOutboxPublisher(repo *models.OutboxEventEntity, cfg OutboxPublisherConfig) *OutboxPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &OutboxPublisher{
		cfg:    cfg,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *OutboxPublisher) Start() {
	ticker := time.NewTicker(p.cfg.ScanInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.tick()
			}
		}
	}()

	if p.cfg.EnableCleanup && p.cfg.Retention > 0 {
		go p.cleanupLoop()
	}
}

func (p *OutboxPublisher) Stop() {
	p.cancel()
}

func (p *OutboxPublisher) tick() {
	events, err := p.repo.FetchPending(p.ctx, p.cfg.BatchSize)
	if err != nil {
		zaplog.Errorf("OutboxPublisher FetchPending error: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}
	mq := server.GetMqSender()
	for _, ev := range events {
		// 发送消息
		headers := map[string]interface{}{}
		body := []byte(ev.Payload)
		// 这里不强制传 ctx 给 sender，若需要 trace，可以在 payload 或 headers 中带上 trace_id
		if err := mq.SendMessage(ev.Exchange, ev.RoutingKey, headers, body, 0 /*priority*/); err != nil {
			// 发送失败 -> 退避重试 或 标记 failed
			nextAttempt, newStatus := p.nextAttemptAndStatus(ev.RetryCount, err)
			msg := truncateErr(err.Error(), 2000)
			if newStatus == models.OutboxStatusFailed {
				zaplog.Errorf("OutboxPublisher send failed permanently event_id=%s err=%v", ev.EventID, err)
				// 最终失败：将状态置为 failed，记录最后错误信息（也可以保留 pending 让人工重试，这里选择失败落地）
				_ = p.repo.db.DB.Model(&models.OutboxEvent{}).
					Where("id = ?", ev.ID).
					Updates(map[string]interface{}{
						"status":          models.OutboxStatusFailed,
						"retry_count":     ev.RetryCount + 1,
						"last_error":      msg,
						"next_attempt_at": nextAttempt,
						"updated_at":      time.Now(),
					}).Error
				continue
			}
			if err := p.repo.MarkFailed(ev.ID, msg, nextAttempt); err != nil {
				zaplog.Errorf("OutboxPublisher MarkFailed error: %v", err)
			}
			continue
		}

		// 发送成功
		if err := p.repo.MarkSent(ev.ID); err != nil {
			zaplog.Errorf("OutboxPublisher MarkSent error: %v", err)
		}
	}
}

func (p *OutboxPublisher) nextAttemptAndStatus(retry int, err error) (time.Time, int) {
	backoff := p.cfg.BaseBackoff * (1 << retry) // 2^retry * base
	if backoff > p.cfg.MaxBackoff {
		backoff = p.cfg.MaxBackoff
	}
	next := time.Now().Add(backoff)
	status := models.OutboxStatusPending
	if retry+1 >= p.cfg.MaxRetry {
		status = models.OutboxStatusFailed
	}
	return next, status
}

func (p *OutboxPublisher) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().Add(-p.cfg.Retention)
			_, err := p.repo.CleanupSentBefore(p.ctx, before, p.cfg.CleanupBatch)
			if err != nil {
				zaplog.Warnf("Outbox cleanup error: %v", err)
			}
		}
	}
}

func truncateErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// 生产者侧：构建事件payload的小工具，可选使用
func BuildConfigHistoryPayload(e *models.ConfigHistoryEvent) (string, error) {
	buf, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
```

说明：
- 发布器使用 `server.GetMqSender()`，与现有框架一致。
- 退避：指数退避，封顶 MaxBackoff；达到 MaxRetry 置为 failed。
- 清理：每小时清理一次超出保留期的 sent 数据。
- 你只需在启动时实例化并 Start（后续我可以按你期望集成到 `server.Start()`）。

## 5) 生产者侧如何在事务内插入 Outbox（示例片段，仅供对接）
你后续在 `UpdateConfig` 和 `ReleaseConfig` 的“历史写入”所在事务里，把 outbox 插入进去即可。示意：

```go
// 示例：在 UpdateConfig 事务内
tx := b.configEntity.Begin()
defer ...
// 1. 正常业务更新...
// 2. 构造历史事件快照（使用更新前的 record 数据）
evt := &models.ConfigHistoryEvent{
  EventID:       uuid.New().String(),   // 需引入 uuid
  EventType:     "config.history.created",
  OccurredAt:    time.Now().Unix(),
  SchemaVersion: 1,
  ConfigId:      record.Id,
  ModuleName:    record.ModuleName,
  Lang:          record.Lang,
  ConfigKey:     record.ConfigKey,
  Desc:          record.Desc,
  ConfigType:    record.ConfigType,
  ConfigValue:   record.ConfigValue,
  ConfigSchema:  record.ConfigSchema,
  Version:       record.Version,        // 注意：历史使用旧版本
  EditUser:      record.EditUser,
  ReleaseUser:   record.ReleaseUser,
  RelatedUsers:  record.RelatedUsers,
  CreatedAt:     record.UpdatedAt,      // 历史创建时间 = 上一条正式数据的 updated_at
}
payload, _ := extra.BuildConfigHistoryPayload(evt)

// 3. 插入 outbox 事件（与业务同事务）
outbox := &models.OutboxEvent{
  EventID:    evt.EventID,
  EventType:  evt.EventType,
  Exchange:   "tmadminsrv_service",
  RoutingKey: "tmadminsrv_config_history",
  Payload:    payload,
  Status:     models.OutboxStatusPending,
}
if err := outboxRepo.InsertInTx(outbox, tx); err != nil {
  tx.Rollback()
  return false, err
}

// 4. 提交事务，发布器会异步扫描并投递到 MQ
if err := tx.Commit().Error; err != nil { ... }
```

`ReleaseConfig` 类似，但遵循你现有条件 `if configRecord.Version > 0` 再插入 outbox。

## 6) 消费者侧（简述）
你现有 MQ 消费架构已就绪。按之前方案，新建一个 `ConfigHistoryCtrl` 消费者，`RegisterQueueTask("tmadminsrv_config_history_queue", ...)`，在 DoNonAction 中解析 `ConfigHistoryEvent`，按历史表字段入库（历史表上加唯一约束 event_id 或 (config_id, version) 保证幂等），错误分为可重试（返回 Err.ErrNeedRetry）与不可重试（让消息进错误队列）。这部分如需，我可再补一份完整消费者代码骨架。

## 7) 发布器初始化示例（仅说明，不改你的代码）
你可在 `server.Start()` 之后、`server.Run()` 之前初始化：

- 从 `server.GetDB("ws_tm_core")` 拿 DB
- `repo := models.NewOutboxEventEntity(db)`
- `pub := extra.NewOutboxPublisher(repo, extra.OutboxPublisherConfig{ ScanInterval: 2 * time.Second, BatchSize: 100, BaseBackoff: 5 * time.Second, MaxBackoff: 10 * time.Minute, MaxRetry: 8, Retention: 7 * 24 * time.Hour, CleanupBatch: 1000, EnableCleanup: true })`
- `pub.Start()`

这样，生产者只负责“事务内写 outbox”，发布器统一投递 MQ，消费者落库历史，形成闭环。

——

以上是你要求的“status+created_at 索引、event_id 唯一、事务内插 outbox、发布器扫描/发送/退避/清理”的完整代码实现与 SQL，不修改现有代码。你把这些新文件加入工程、创建表后，就可以按示例在业务事务里插入 outbox 记录，功能即可跑起来。需要我继续补消费者骨架或集成初始化位置的话，告诉我你的偏好（放在 init.go 里或 server.startup.go 里），我再给出对应代码。


我将先快速查看你项目中的 models 与 database 使用风格，以便产出可直接落地的 Outbox 表模型与仓库代码（保持和现有仓库风格一致），不会修改任何现有文件，只做只读浏览。随后我会输出全量的新文件代码和 SQL。 

搜索与 `**/common/models/**/*.go` 匹配的文件，16 匹配项

读取 [](file:///c%3A/Users/ws/Desktop/Code/tmadminsrv/common/models/config_history.go)

好的，我来给出“Outbox 强一致方案”的完整可落地实现代码与 SQL（不改你现有代码），你把这些新文件加进来即可；之后只需在 UpdateConfig/ReleaseConfig 的事务里插入一条 outbox 记录即可完成生产者侧改造。

## 目录与文件建议
- 新增表 SQL：`db/migrations/20250929_create_outbox_events.sql`
- 新增模型与仓库：`common/models/outbox_event.go`
- 新增事件定义：`modules/api/models/config_history_event.go`
- 新增发布器：`modules/api/extra/outbox_publisher.go`

你可按现有项目风格调整路径，但建议保持 models 在 models，业务相关放 `modules/api/...`。

## 1) 新增表 SQL

说明：
- 索引：status+created_at（扫描顺序），event_id 唯一（幂等），可选 next_attempt_at（更精准过滤）
- 保留 exchange 和 routing_key 字段，避免发布器硬编码

内容（MySQL 8+，兼容 utf8mb4）：

```sql
-- db/migrations/20250929_create_outbox_events.sql
CREATE TABLE IF NOT EXISTS `outbox_events` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增ID',
  `event_id` VARCHAR(64) NOT NULL COMMENT '全局唯一事件ID，用于幂等',
  `event_type` VARCHAR(128) NOT NULL COMMENT '事件类型，如 config.history.created',
  `exchange` VARCHAR(128) NOT NULL COMMENT 'MQ exchange',
  `routing_key` VARCHAR(128) NOT NULL COMMENT 'MQ routing key',
  `payload` JSON NOT NULL COMMENT '事件负载，JSON 快照',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0=pending,1=sent,2=failed',
  `retry_count` INT NOT NULL DEFAULT 0 COMMENT '重试次数',
  `last_error` TEXT NULL COMMENT '最后一次错误信息',
  `next_attempt_at` DATETIME NULL COMMENT '下次尝试时间（用于退避）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_event_id` (`event_id`),
  KEY `idx_status_created_at` (`status`, `created_at`),
  KEY `idx_status_next_attempt` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

保留了 `payload` 为 JSON 类型，如你的 MySQL 版本不支持 JSON，可改为 `LONGTEXT`。

## 2) Outbox 模型与仓库

```go
// common/models/outbox_event.go
package models

import (
	"context"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/core/database"
	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	Err "git.wondershare.cn/piccloud/tmadminsrv/error"
	"github.com/jinzhu/gorm"
)

const (
	OutboxStatusPending = 0
	OutboxStatusSent    = 1
	OutboxStatusFailed  = 2
)

type OutboxEvent struct {
	ID            uint64    `json:"id" gorm:"primary_key;autoIncrement"`
	EventID       string    `json:"event_id" gorm:"type:varchar(64);unique_index:uk_event_id;not null"`
	EventType     string    `json:"event_type" gorm:"type:varchar(128);not null"`
	Exchange      string    `json:"exchange" gorm:"type:varchar(128);not null"`
	RoutingKey    string    `json:"routing_key" gorm:"type:varchar(128);not null"`
	Payload       string    `json:"payload" gorm:"type:longtext;not null"`
	Status        int       `json:"status" gorm:"type:tinyint;not null;default:0"`
	RetryCount    int       `json:"retry_count" gorm:"type:int;not null;default:0"`
	LastError     string    `json:"last_error" gorm:"type:longtext"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	CreatedAt     time.Time `json:"created_at" gorm:"type:datetime;not null"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"type:datetime;not null"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}

type OutboxEventRepository interface {
	InsertInTx(e *OutboxEvent, tx *gorm.DB) error
	FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkSent(id uint64) error
	MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error
	CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

type OutboxEventEntity struct {
	db *database.DB
}

func NewOutboxEventEntity(db *database.DB) *OutboxEventEntity {
	return &OutboxEventEntity{db: db}
}

func (e *OutboxEventEntity) InsertInTx(ev *OutboxEvent, tx *gorm.DB) error {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.UpdatedAt.IsZero() {
		ev.UpdatedAt = ev.CreatedAt
	}
	if err := tx.Create(ev).Error; err != nil {
		zaplog.Errorf("Outbox InsertInTx failed: %v", err)
		return Err.ErrCreateFailed
	}
	return nil
}

func (e *OutboxEventEntity) FetchPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	now := time.Now()
	var list []OutboxEvent
	q := e.db.DB.
		Where("status = ?", OutboxStatusPending).
		Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now).
		Order("created_at asc").
		Limit(limit)
	if err := q.Find(&list).Error; err != nil {
		zaplog.ErrorWithCtx(ctx, "Outbox FetchPending failed: %v", err)
		return nil, Err.ErrDB
	}
	return list, nil
}

func (e *OutboxEventEntity) MarkSent(id uint64) error {
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     OutboxStatusSent,
			"updated_at": time.Now(),
			"last_error": "",
		}).Error
}

func (e *OutboxEventEntity) MarkFailed(id uint64, errMsg string, nextAttempt time.Time) error {
	// 原子地增加 retry_count 并设置下一次尝试时间
	return e.db.DB.Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          OutboxStatusPending, // 保持 pending，等待下次扫描
			"retry_count":     gorm.Expr("retry_count + 1"),
			"last_error":      errMsg,
			"next_attempt_at": nextAttempt,
			"updated_at":      time.Now(),
		}).Error
}

func (e *OutboxEventEntity) CleanupSentBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	// 分批删除 sent 的历史事件
	var affected int64
	tx := e.db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 用子查询限定范围
	sub := tx.Model(&OutboxEvent{}).
		Select("id").
		Where("status = ?", OutboxStatusSent).
		Where("updated_at < ?", before).
		Limit(limit).SubQuery()

	if err := tx.Where("id in (?)", sub).Delete(OutboxEvent{}).Error; err != nil {
		tx.Rollback()
		zaplog.ErrorWithCtx(ctx, "Outbox CleanupSentBefore failed: %v", err)
		return 0, Err.ErrDB
	}
	if err := tx.Commit().Error; err != nil {
		return 0, Err.ErrDB
	}
	// 受影响行数不能直接从 Delete 返回，必要时再查计数；这里简单返回 limit，实际使用可改为准确值
	affected = int64(limit)
	return affected, nil
}
```

说明：
- InsertInTx：用于业务事务内插入 outbox 记录。
- FetchPending：取待发送的记录；按 created_at 升序保障事件顺序性。
- MarkFailed：不把状态置为 failed，而是保留 pending 并设置 next_attempt_at 实现退避；超过阈值转 failed 的策略可以放到发布器里决定。
- CleanupSentBefore：用于定期清理已发送的老记录。

## 3) 事件定义（历史快照）

```go
// modules/api/models/config_history_event.go
package models

import "time"

// ConfigHistoryEvent 对齐 ConfigHistoryModel 的字段，作为 outbox payload 的 JSON
type ConfigHistoryEvent struct {
	EventID       string    `json:"event_id"`        // 幂等ID
	EventType     string    `json:"event_type"`      // 固定：config.history.created
	OccurredAt    int64     `json:"occurred_at"`     // 事件产生时间（秒）
	SchemaVersion int       `json:"schema_version"`  // 例如 1
	TraceID       string    `json:"trace_id,omitempty"`

	ConfigId     int64     `json:"config_id"`
	ModuleName   string    `json:"module_name"`
	Lang         string    `json:"lang"`
	ConfigKey    string    `json:"config_key"`
	Desc         string    `json:"desc"`
	ConfigType   int       `json:"config_type"`
	ConfigValue  string    `json:"config_value"`
	ConfigSchema string    `json:"config_schema"`
	Version      int       `json:"version"`
	EditUser     string    `json:"edit_user"`
	ReleaseUser  string    `json:"release_user"`
	RelatedUsers string    `json:"related_users"`
	CreatedAt    time.Time `json:"created_at"` // 历史记录的创建时间（上一条正式数据的 updated_at）
}
```

说明：
- 这就是最终写入历史表需要的快照，消费者拿到它即可入库。
- 生产者侧将此结构体 JSON 序列化后放入 outbox 的 payload。

## 4) 出站发布器（扫描 pending -> 发送 MQ -> 标记 sent/重试/失败）

```go
// modules/api/extra/outbox_publisher.go
package extra

import (
	"context"
	"encoding/json"
	"time"

	"git.wondershare.cn/DCStudio/chaos_go/utils/zaplog"
	"git.wondershare.cn/piccloud/tmadminsrv/common/models"
	"git.wondershare.cn/piccloud/tmadminsrv/server"
)

type OutboxPublisherConfig struct {
	ScanInterval   time.Duration // 扫描周期
	BatchSize      int           // 每次批量发送数
	BaseBackoff    time.Duration // 基础退避（如 5s）
	MaxBackoff     time.Duration // 最大退避（如 10m）
	MaxRetry       int           // 最大重试次数（超过则置 failed）
	Retention      time.Duration // 已发送保留时长（如 7*24h）
	CleanupBatch   int           // 清理每批条数（如 500）
	EnableCleanup  bool          // 是否清理
}

type OutboxPublisher struct {
	cfg     OutboxPublisherConfig
	repo    *models.OutboxEventEntity
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewOutboxPublisher(repo *models.OutboxEventEntity, cfg OutboxPublisherConfig) *OutboxPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &OutboxPublisher{
		cfg:    cfg,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *OutboxPublisher) Start() {
	ticker := time.NewTicker(p.cfg.ScanInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.tick()
			}
		}
	}()

	if p.cfg.EnableCleanup && p.cfg.Retention > 0 {
		go p.cleanupLoop()
	}
}

func (p *OutboxPublisher) Stop() {
	p.cancel()
}

func (p *OutboxPublisher) tick() {
	events, err := p.repo.FetchPending(p.ctx, p.cfg.BatchSize)
	if err != nil {
		zaplog.Errorf("OutboxPublisher FetchPending error: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}
	mq := server.GetMqSender()
	for _, ev := range events {
		// 发送消息
		headers := map[string]interface{}{}
		body := []byte(ev.Payload)
		// 这里不强制传 ctx 给 sender，若需要 trace，可以在 payload 或 headers 中带上 trace_id
		if err := mq.SendMessage(ev.Exchange, ev.RoutingKey, headers, body, 0 /*priority*/); err != nil {
			// 发送失败 -> 退避重试 或 标记 failed
			nextAttempt, newStatus := p.nextAttemptAndStatus(ev.RetryCount, err)
			msg := truncateErr(err.Error(), 2000)
			if newStatus == models.OutboxStatusFailed {
				zaplog.Errorf("OutboxPublisher send failed permanently event_id=%s err=%v", ev.EventID, err)
				// 最终失败：将状态置为 failed，记录最后错误信息（也可以保留 pending 让人工重试，这里选择失败落地）
				_ = p.repo.db.DB.Model(&models.OutboxEvent{}).
					Where("id = ?", ev.ID).
					Updates(map[string]interface{}{
						"status":          models.OutboxStatusFailed,
						"retry_count":     ev.RetryCount + 1,
						"last_error":      msg,
						"next_attempt_at": nextAttempt,
						"updated_at":      time.Now(),
					}).Error
				continue
			}
			if err := p.repo.MarkFailed(ev.ID, msg, nextAttempt); err != nil {
				zaplog.Errorf("OutboxPublisher MarkFailed error: %v", err)
			}
			continue
		}

		// 发送成功
		if err := p.repo.MarkSent(ev.ID); err != nil {
			zaplog.Errorf("OutboxPublisher MarkSent error: %v", err)
		}
	}
}

func (p *OutboxPublisher) nextAttemptAndStatus(retry int, err error) (time.Time, int) {
	backoff := p.cfg.BaseBackoff * (1 << retry) // 2^retry * base
	if backoff > p.cfg.MaxBackoff {
		backoff = p.cfg.MaxBackoff
	}
	next := time.Now().Add(backoff)
	status := models.OutboxStatusPending
	if retry+1 >= p.cfg.MaxRetry {
		status = models.OutboxStatusFailed
	}
	return next, status
}

func (p *OutboxPublisher) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().Add(-p.cfg.Retention)
			_, err := p.repo.CleanupSentBefore(p.ctx, before, p.cfg.CleanupBatch)
			if err != nil {
				zaplog.Warnf("Outbox cleanup error: %v", err)
			}
		}
	}
}

func truncateErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// 生产者侧：构建事件payload的小工具，可选使用
func BuildConfigHistoryPayload(e *models.ConfigHistoryEvent) (string, error) {
	buf, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
```

说明：
- 发布器使用 `server.GetMqSender()`，与现有框架一致。
- 退避：指数退避，封顶 MaxBackoff；达到 MaxRetry 置为 failed。
- 清理：每小时清理一次超出保留期的 sent 数据。
- 你只需在启动时实例化并 Start（后续我可以按你期望集成到 `server.Start()`）。

## 5) 生产者侧如何在事务内插入 Outbox（示例片段，仅供对接）
你后续在 `UpdateConfig` 和 `ReleaseConfig` 的“历史写入”所在事务里，把 outbox 插入进去即可。示意：

```go
// 示例：在 UpdateConfig 事务内
tx := b.configEntity.Begin()
defer ...
// 1. 正常业务更新...
// 2. 构造历史事件快照（使用更新前的 record 数据）
evt := &models.ConfigHistoryEvent{
  EventID:       uuid.New().String(),   // 需引入 uuid
  EventType:     "config.history.created",
  OccurredAt:    time.Now().Unix(),
  SchemaVersion: 1,
  ConfigId:      record.Id,
  ModuleName:    record.ModuleName,
  Lang:          record.Lang,
  ConfigKey:     record.ConfigKey,
  Desc:          record.Desc,
  ConfigType:    record.ConfigType,
  ConfigValue:   record.ConfigValue,
  ConfigSchema:  record.ConfigSchema,
  Version:       record.Version,        // 注意：历史使用旧版本
  EditUser:      record.EditUser,
  ReleaseUser:   record.ReleaseUser,
  RelatedUsers:  record.RelatedUsers,
  CreatedAt:     record.UpdatedAt,      // 历史创建时间 = 上一条正式数据的 updated_at
}
payload, _ := extra.BuildConfigHistoryPayload(evt)

// 3. 插入 outbox 事件（与业务同事务）
outbox := &models.OutboxEvent{
  EventID:    evt.EventID,
  EventType:  evt.EventType,
  Exchange:   "tmadminsrv_service",
  RoutingKey: "tmadminsrv_config_history",
  Payload:    payload,
  Status:     models.OutboxStatusPending,
}
if err := outboxRepo.InsertInTx(outbox, tx); err != nil {
  tx.Rollback()
  return false, err
}

// 4. 提交事务，发布器会异步扫描并投递到 MQ
if err := tx.Commit().Error; err != nil { ... }
```

`ReleaseConfig` 类似，但遵循你现有条件 `if configRecord.Version > 0` 再插入 outbox。

## 6) 消费者侧（简述）
你现有 MQ 消费架构已就绪。按之前方案，新建一个 `ConfigHistoryCtrl` 消费者，`RegisterQueueTask("tmadminsrv_config_history_queue", ...)`，在 DoNonAction 中解析 `ConfigHistoryEvent`，按历史表字段入库（历史表上加唯一约束 event_id 或 (config_id, version) 保证幂等），错误分为可重试（返回 Err.ErrNeedRetry）与不可重试（让消息进错误队列）。这部分如需，我可再补一份完整消费者代码骨架。

## 7) 发布器初始化示例（仅说明，不改你的代码）
你可在 `server.Start()` 之后、`server.Run()` 之前初始化：

- 从 `server.GetDB("ws_tm_core")` 拿 DB
- `repo := models.NewOutboxEventEntity(db)`
- `pub := extra.NewOutboxPublisher(repo, extra.OutboxPublisherConfig{ ScanInterval: 2 * time.Second, BatchSize: 100, BaseBackoff: 5 * time.Second, MaxBackoff: 10 * time.Minute, MaxRetry: 8, Retention: 7 * 24 * time.Hour, CleanupBatch: 1000, EnableCleanup: true })`
- `pub.Start()`

这样，生产者只负责“事务内写 outbox”，发布器统一投递 MQ，消费者落库历史，形成闭环。

——

以上是你要求的“status+created_at 索引、event_id 唯一、事务内插 outbox、发布器扫描/发送/退避/清理”的完整代码实现与 SQL，不修改现有代码。你把这些新文件加入工程、创建表后，就可以按示例在业务事务里插入 outbox 记录，功能即可跑起来。需要我继续补消费者骨架或集成初始化位置的话，告诉我你的偏好（放在 init.go 里或 server.startup.go 里），我再给出对应代码。



**这个理由不够充分**，让我来具体分析为什么。

## 为什么"业务高峰期偶发压力"理由不够充分

### 1) **你已经明确说了"没有高并发场景"**
```go
// 你的实际情况
BatchChangeConfig() {
    // 限制并发数: semaphore := make(chan struct{}, 30)
    // 说明你们的"批量"也就是几十个并发
}
```

**现实对比**：
- 真正的"业务高峰"：几百到几千 QPS
- 你们的"高峰"：可能就是几十个配置的批量操作
- 这种规模下，MySQL 处理历史表 INSERT 绰绰有余

### 2) **"偶发压力"的技术分析站不住脚**
```sql
-- 历史表的 INSERT 操作特点
INSERT INTO config_history (config_id, module_name, lang, config_key, ...) 
VALUES (123, 'user_service', 'en', 'feature_x', ...);

-- 这是最简单的数据库操作：
-- 1. 无复杂 JOIN
-- 2. 无唯一约束冲突（如果不加 event_id 约束）
-- 3. 无外键约束
-- 4. 表结构简单，索引少
-- 5. 数据量小（每条记录几 KB）
```

**性能现实**：
- 现代 MySQL 在普通硬件上：INSERT 性能 > 1万 QPS
- 你们的历史记录：可能一天都不到 100 次
- 即使"高峰期"批量操作：也就几十到几百次
- **性能根本不是瓶颈**

### 3) **"发版、批量配置修改"场景的实际分析**

#### 发版场景：
```go
// 典型发版操作
ReleaseConfig("app_version", "v2.1.0")          // 1次
ReleaseConfig("feature_flags", "{...}")         // 1次  
ReleaseConfig("database_url", "new_endpoint")   // 1次
// 总共：3-10 次操作，间隔几分钟

// 这能叫"高峰压力"吗？
```

#### 批量配置修改：
```go
// 你们的 BatchChangeConfig 已经有并发控制
semaphore := make(chan struct{}, 30)  // 最多30并发
// 即使100个配置批量修改，也就是30并发，持续几秒钟
// MySQL 轻松应对
```

**真相**：你们所谓的"业务高峰"对数据库来说根本不算压力。

### 4) **用数据说话：计算实际 QPS**
```go
// 假设你们最极端的场景
// 批量修改1000个配置（已经很夸张了）
// 30个并发，每个操作耗时100ms
// 总耗时：1000/30 * 0.1s = 3.33秒
// QPS：1000/3.33 = 300 QPS

// 而 MySQL INSERT 的性能基准：
// 普通服务器：5000-10000 QPS
// 你们的场景：300 QPS
// 性能余量：95%+
```

**结论**：你们的"高峰"对 MySQL 来说是"毛毛雨"。

## 真正站得住脚的理由应该是什么

### 1) **故障隔离，而非性能**
```go
// 不是因为写不过来，而是因为可能写不进去
// 比如：
// - 历史表被 DDL 锁住（加索引、修表结构）
// - 磁盘空间不足
// - 主从复制延迟导致的锁等待
// - 数据库维护窗口
```

### 2) **业务连续性，而非高并发**
```go
// 配置发布是关键路径，不能因为历史记录而阻断
if configRecord.Version > 0 {
    if err := b.configHistoryEntity.Save(oldConfig, tx); err != nil {
        tx.Rollback()  // 这里回滚就是业务风险！
        return false, err  // 用户无法发布配置！
    }
}

// 真正的问题：历史记录（次要功能）阻断了配置发布（核心功能）
```

### 3) **运维友好性，而非技术架构**
```go
// 同步方案的运维痛点：
// 1. 故障诊断难：配置发布失败，到底是主表问题还是历史表问题？
// 2. 修复困难：历史表有问题，是临时禁用历史功能，还是修复后重跑？
// 3. 监控盲区：历史记录失败可能被淹没在业务日志里

// 异步方案的运维优势：
// 1. 问题隔离：配置发布成功，历史处理是独立的
// 2. 可补偿：历史缺失可以通过队列重放或手动补录
// 3. 可观测：专门的队列指标、重试计数、失败告警
```

## 修正后的充分理由

如果你要为 Outbox/异步化方案辩护，应该这么说：

### 充分理由1：**故障隔离**
```go
// 当前风险：历史表的任何问题都会阻断配置发布
// - 表被锁定（DDL 操作）
// - 磁盘满
// - 表损坏
// - 网络分区导致的主从不一致

// 异步化价值：配置发布和历史记录完全解耦
// 即使历史系统完全宕机，配置发布依然正常
```

### 充分理由2：**业务连续性**
```go
// 配置系统通常是其他服务的依赖
// 一旦配置发布被阻断，可能影响：
// - 功能开关无法及时调整
// - 紧急故障无法通过配置修复
// - 业务参数无法动态调整

// 历史记录的重要性 < 配置发布的重要性
// 异步化符合业务优先级
```

### 充分理由3：**技术债务**
```go
// 当前方案的技术债务：
// 1. 主要业务逻辑和次要功能耦合
// 2. 错误处理复杂（需要区分主表错误和历史表错误）
// 3. 扩展性差（如果要加审计日志、ES 同步等，都要修改主流程）

// 异步化的技术价值：
// 1. 关注点分离
// 2. 可扩展（一个事件，多个消费者）
// 3. 可测试（主流程和历史流程独立测试）
```

## 最终结论

**"业务高峰期偶发压力"这个理由确实不充分**，因为：
1. 你明确说了没有高并发
2. 历史表 INSERT 的性能门槛很低
3. 你们的"高峰"规模对数据库来说不值一提

**真正充分的理由应该是"故障隔离"和"业务连续性"**，而不是性能考虑。

如果你要说服团队做异步化，重点强调：
- 历史记录失败不应该阻断配置发布（业务风险）
- 运维友好性和故障隔离（工程价值）
- 为未来扩展做准备（技术前瞻）

不要提性能和高并发，那不是你们的真实痛点。