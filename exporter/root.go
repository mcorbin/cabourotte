package exporter

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gopkg.in/tomb.v2"

	"cabourotte/healthcheck"
	"cabourotte/memorystore"
)

// Exporter the exporter interface
type Exporter interface {
	Start() error
	Stop() error
	Push(*healthcheck.Result) error
}

// Component the exporter component
type Component struct {
	Logger      *zap.Logger
	Config      *Configuration
	ChanResult  chan *healthcheck.Result
	Exporters   []Exporter
	MemoryStore *memorystore.MemoryStore

	t tomb.Tomb
}

// New creates a new exporter component
func New(logger *zap.Logger, store *memorystore.MemoryStore, chanResult chan *healthcheck.Result, config *Configuration) *Component {
	var exporters []Exporter
	for _, httpConfig := range config.HTTP {
		exporters = append(exporters, NewHTTPExporter(logger, &httpConfig))
	}
	return &Component{
		MemoryStore: store,
		Logger:      logger,
		Config:      config,
		ChanResult:  chanResult,
		Exporters:   exporters,
	}
}

// Start starts the exporter component
func (c *Component) Start() error {
	c.Logger.Info("Starting the exporters")
	c.MemoryStore.Start()
	c.t.Go(func() error {
		for {
			select {
			case message := <-c.ChanResult:
				c.MemoryStore.Add(message)
				if message.Success {
					c.Logger.Info("Healthcheck successful",
						zap.String("name", message.Name),
						zap.String("date", message.Timestamp.String()),
					)
				} else {
					c.Logger.Info("healthcheck failed",
						zap.String("name", message.Name),
						zap.String("extra", message.Message),
						zap.String("date", message.Timestamp.String()),
					)
				}
				for _, exporter := range c.Exporters {
					exporter.Push(message)
				}
			case <-c.t.Dying():
				return nil
			}
		}
	})
	// nothing to do
	return nil
}

// Stop the exporters
func (c *Component) Stop() error {
	c.t.Kill(nil)
	c.t.Wait()
	for _, e := range c.Exporters {
		err := e.Stop()
		if err != nil {
			return errors.Wrapf(err, "Fail to stop an exporter")
		}
	}
	return nil
}