package verifyprocessor

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"audit-loss/collector/components/hmac"
)

func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType("verify"),
		func() component.Config { return &Config{} },
		processor.WithLogs(createLogs, component.StabilityLevelAlpha),
	)
}

func createLogs(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	c := cfg.(*Config)
	key, err := hmac.LoadKey(c.KeyFile)
	if err != nil {
		return nil, err
	}
	p := &verifyProc{
		key:    key,
		sink:   c.SinkURL,
		client: &http.Client{Timeout: 5 * time.Second},
	}
	return processorhelper.NewLogs(
		ctx,
		set,
		cfg,
		next,
		p.process,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
