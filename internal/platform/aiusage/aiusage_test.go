package aiusage

import "testing"

func TestEstimateCost(t *testing.T) {
	service := &Service{prices: Prices{
		InputPerMillion:     2,
		OutputPerMillion:    8,
		EmbeddingPerMillion: 0.1,
	}}

	if got := service.EstimateCost(OperationCompanion, 1_000_000, 500_000); got != 6 {
		t.Errorf("companion cost = %v, want 6", got)
	}
	if got := service.EstimateCost(OperationInsight, 500_000, 0); got != 1 {
		t.Errorf("insight cost = %v, want 1", got)
	}
	if got := service.EstimateCost(OperationEmbedding, 2_000_000, 0); got != 0.2 {
		t.Errorf("embedding cost = %v, want 0.2", got)
	}
	if got := service.EstimateCost(OperationCompanion, 0, 0); got != 0 {
		t.Errorf("zero cost = %v, want 0", got)
	}
}
