package slog

import "github.com/darkit/slog/modules"

// ModuleDiagnostics 描述模块健康与指标信息。
type ModuleDiagnostics struct {
	Name     string             `json:"name"`
	Type     modules.ModuleType `json:"type"`
	Enabled  bool               `json:"enabled"`
	Healthy  *bool              `json:"healthy,omitempty"`
	Metrics  map[string]any     `json:"metrics,omitempty"`
	Priority int                `json:"priority"`
}

// CollectModuleDiagnostics 聚合已注册模块的健康状态与指标。
func CollectModuleDiagnostics() []ModuleDiagnostics {
	return GetGlobalLogger().Diagnostics()
}

func collectModuleDiagnostics(moduleList []modules.Module) []ModuleDiagnostics {
	diags := make([]ModuleDiagnostics, 0, len(moduleList))
	for _, m := range moduleList {
		diag := ModuleDiagnostics{
			Name:     m.Name(),
			Type:     m.Type(),
			Enabled:  m.Enabled(),
			Priority: m.Priority(),
		}
		if healthable, ok := m.(interface {
			HealthCheck() error
			IsHealthy() bool
		}); ok {
			err := healthable.HealthCheck()
			healthy := err == nil && healthable.IsHealthy()
			diag.Healthy = &healthy
		}
		if measurable, ok := m.(interface{ GetMetrics() map[string]any }); ok {
			diag.Metrics = measurable.GetMetrics()
		}
		diags = append(diags, diag)
	}
	return diags
}
