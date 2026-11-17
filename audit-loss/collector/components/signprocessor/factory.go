package signprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"

	"audit-loss/collector/components/hmac"
)

func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType("sign"),
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
	return processorhelper.NewLogs(
		ctx,
		set,
		cfg,
		next,
		(&signProc{key: key}).process,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}
