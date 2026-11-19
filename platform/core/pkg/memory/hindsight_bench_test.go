package memory

import (
	"testing"
)

func benchStore(b *testing.B) *HindsightStore {
	b.Helper()
	cfg := DefaultStoreConfig()
	cfg.MaxItemsPerBank = 1000
	cfg.MaxTotalItems = 5000
	s := NewHindsightStoreWithConfig(cfg)
	for i := 0; i < 500; i++ {
		s.RetainScoped("lab-a", "lab-a", BankExperiences, "experience about campaign optimisation and tools", 0.5, nil)
	}
	return s
}

func BenchmarkHindsightRecall500(b *testing.B) {
	s := benchStore(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.SemanticRecallScoped(BankExperiences, "campaign optimisation", 250, 0.2, "lab-a", []string{"lab-a"})
	}
}

func BenchmarkHindsightRetain(b *testing.B) {
	cfg := DefaultStoreConfig()
	cfg.MaxItemsPerBank = 100000
	cfg.MaxTotalItems = 100000
	s := NewHindsightStoreWithConfig(cfg)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.RetainScoped("lab-a", "lab-a", BankExperiences, "unique experience content for benchmark", 0.5, nil)
	}
}
