package event

import (
	"database/sql"

	wsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	_ "github.com/go-sql-driver/mysql"
)

// MySQLConfig MySQL 配置
type MySQLConfig struct {
	DSN string
}

// MySQLDriver MySQL 驱动
type MySQLDriver struct {
	db         *sql.DB
	publisher  *wsql.Publisher
	subscriber *wsql.Subscriber
	ownDB      bool
}

// NewMySQLDriver 从配置创建 MySQL 驱动
func NewMySQLDriver(cfg MySQLConfig, opts ...DriverOption) (*MySQLDriver, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	driver, err := newMySQLDriverFromDB(db, opts...)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	driver.ownDB = true
	return driver, nil
}

// NewMySQLDriverFromDB 从现有数据库连接创建 MySQL 驱动
func NewMySQLDriverFromDB(db *sql.DB, opts ...DriverOption) (*MySQLDriver, error) {
	return newMySQLDriverFromDB(db, opts...)
}

func newMySQLDriverFromDB(db *sql.DB, opts ...DriverOption) (*MySQLDriver, error) {
	options := applyDriverOptions(opts)

	publisher, err := wsql.NewPublisher(
		wsql.BeginnerFromStdSQL(db),
		wsql.PublisherConfig{
			SchemaAdapter:        wsql.DefaultMySQLSchema{},
			AutoInitializeSchema: true,
		}, options.logger)
	if err != nil {
		return nil, err
	}

	subscriber, err := wsql.NewSubscriber(
		wsql.BeginnerFromStdSQL(db),
		wsql.SubscriberConfig{
			SchemaAdapter:    wsql.DefaultMySQLSchema{},
			OffsetsAdapter:   wsql.DefaultMySQLOffsetsAdapter{},
			InitializeSchema: true,
		}, options.logger)
	if err != nil {
		_ = publisher.Close()
		return nil, err
	}

	return &MySQLDriver{
		db:         db,
		publisher:  publisher,
		subscriber: subscriber,
		ownDB:      false,
	}, nil
}

func (d *MySQLDriver) Publisher() message.Publisher   { return d.publisher }
func (d *MySQLDriver) Subscriber() message.Subscriber { return d.subscriber }

func (d *MySQLDriver) Close() error {
	_ = d.publisher.Close()
	_ = d.subscriber.Close()
	if d.ownDB {
		return d.db.Close()
	}
	return nil
}
