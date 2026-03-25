package event

import (
	"database/sql"

	wsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	_ "github.com/lib/pq"
)

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	DSN string
}

// PostgresDriver PostgreSQL 驱动
type PostgresDriver struct {
	db         *sql.DB
	publisher  *wsql.Publisher
	subscriber *wsql.Subscriber
	ownDB      bool
}

// NewPostgresDriver 从配置创建 PostgreSQL 驱动
func NewPostgresDriver(cfg PostgresConfig, opts ...DriverOption) (*PostgresDriver, error) {
	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	driver, err := newPostgresDriverFromDB(db, opts...)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	driver.ownDB = true
	return driver, nil
}

// NewPostgresDriverFromDB 从现有数据库连接创建 PostgreSQL 驱动
func NewPostgresDriverFromDB(db *sql.DB, opts ...DriverOption) (*PostgresDriver, error) {
	return newPostgresDriverFromDB(db, opts...)
}

func newPostgresDriverFromDB(db *sql.DB, opts ...DriverOption) (*PostgresDriver, error) {
	options := applyDriverOptions(opts)

	publisher, err := wsql.NewPublisher(
		wsql.BeginnerFromStdSQL(db),
		wsql.PublisherConfig{
			SchemaAdapter:        wsql.DefaultPostgreSQLSchema{},
			AutoInitializeSchema: true,
		}, options.logger)
	if err != nil {
		return nil, err
	}

	subscriber, err := wsql.NewSubscriber(
		wsql.BeginnerFromStdSQL(db),
		wsql.SubscriberConfig{
			SchemaAdapter:    wsql.DefaultPostgreSQLSchema{},
			OffsetsAdapter:   wsql.DefaultPostgreSQLOffsetsAdapter{},
			InitializeSchema: true,
		}, options.logger)
	if err != nil {
		_ = publisher.Close()
		return nil, err
	}

	return &PostgresDriver{
		db:         db,
		publisher:  publisher,
		subscriber: subscriber,
		ownDB:      false,
	}, nil
}

func (d *PostgresDriver) Publisher() message.Publisher   { return d.publisher }
func (d *PostgresDriver) Subscriber() message.Subscriber { return d.subscriber }

func (d *PostgresDriver) Close() error {
	_ = d.publisher.Close()
	_ = d.subscriber.Close()
	if d.ownDB {
		return d.db.Close()
	}
	return nil
}
