package disk

// S2 — disk guard tests. Threat: unmodelled disk-full outage. The guard
// must trip hysteresis bands exactly, fail closed on unmeasurable disks,
// and take the minimum across watch paths (guarding / while state lives
// elsewhere is fiction).

import (
	"errors"
	"testing"
)

func scripted(values []uint64, errs []bool) func(string) (uint64, error) {
	i := 0
	return func(path string) (uint64, error) {
		if i >= len(values) {
			i = len(values) - 1
		}
		v, e := values[i], errs[i]
		i++
		if e {
			return 0, errors.New("statfs: no such file or directory")
		}
		return v, nil
	}
}

const (
	gb = 1024 * 1024 * 1024
)

func TestThresholdValidation(t *testing.T) {
	if err := DefaultThresholds().Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	for _, bad := range []Thresholds{
		{WarnBytes: 0, BlockBytes: 1, ResumeBytes: 1},
		{WarnBytes: 5 * gb, BlockBytes: 5 * gb, ResumeBytes: 5 * gb},
		{WarnBytes: 1 * gb, BlockBytes: 2 * gb, ResumeBytes: 3 * gb}, // inverted
		{WarnBytes: 5 * gb, BlockBytes: 3 * gb, ResumeBytes: 2 * gb}, // resume < block
		{WarnBytes: 5 * gb, BlockBytes: -1, ResumeBytes: 2 * gb},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("must refuse %+v", bad)
		}
	}
}

func TestThresholdsFromEnv(t *testing.T) {
	t.Setenv("KUBONA_DISK_WARN_BYTES", "4294967296")
	t.Setenv("KUBONA_DISK_BLOCK_BYTES", "1073741824")
	t.Setenv("KUBONA_DISK_RESUME_BYTES", "1610612736")
	th, err := ThresholdsFromEnv()
	if err != nil {
		t.Fatalf("env thresholds: %v", err)
	}
	if th.WarnBytes != 4294967296 || th.BlockBytes != 1073741824 || th.ResumeBytes != 1610612736 {
		t.Fatalf("env not honored: %+v", th)
	}
	t.Setenv("KUBONA_DISK_BLOCK_BYTES", "not-a-number")
	if _, err := ThresholdsFromEnv(); err == nil {
		t.Fatalf("malformed env must error (boot refuses)")
	}
}

func TestHysteresisBands(t *testing.T) {
	// Sequence (GB): ok → warn → blocked → hold → resume → warn → hold → clear.
	seq := []uint64{10 * gb, 1500 * 1024 * 1024, 512 * 1024 * 1024, 1200 * 1024 * 1024, 1600 * 1024 * 1024, 1500 * 1024 * 1024, 2100 * 1024 * 1024, 2500 * 1024 * 1024}
	want := []string{LevelOK, LevelWarning, LevelBlocked, LevelBlocked, LevelOK, LevelWarning, LevelWarning, LevelOK}
	var transitions []string
	g := NewGuard(DefaultThresholds(), []string{"/state"}, scripted(seq, make([]bool, len(seq))), func(s Snapshot) {
		transitions = append(transitions, s.Level)
	})
	for i, w := range want {
		g.PollOnce()
		if got := g.Snapshot().Level; got != w {
			t.Fatalf("step %d: want %s got %s", i, w, got)
		}
	}
	if g.Blocks() {
		t.Fatalf("must end unblocked")
	}
	// Transitions fire only on change: ok→warn→blocked→ok→warn→ok = 5.
	if len(transitions) != 5 {
		t.Fatalf("want 5 transitions, got %v", transitions)
	}
}

func TestUnmeasurableBlocksFailClosed(t *testing.T) {
	g := NewGuard(DefaultThresholds(), []string{"/missing-mount"}, scripted([]uint64{0}, []bool{true}), nil)
	g.PollOnce()
	snap := g.Snapshot()
	if snap.Level != LevelBlocked || !g.Blocks() {
		t.Fatalf("unmeasurable disk must block: %+v", snap)
	}
	if snap.Reason == "" {
		t.Fatalf("refusal must name the path/cause")
	}
}

func TestMinAcrossMountsWins(t *testing.T) {
	calls := 0
	g := NewGuard(DefaultThresholds(), []string{"/big", "/small"}, func(p string) (uint64, error) {
		calls++
		if p == "/small" {
			return 512 * 1024 * 1024, nil
		}
		return 100 * gb, nil
	}, nil)
	g.PollOnce()
	if !g.Blocks() {
		t.Fatalf("one full mount must block even when others are empty")
	}
	if snap := g.Snapshot(); snap.AvailBytes != 512*1024*1024 {
		t.Fatalf("snapshot must carry the minimum, got %d", snap.AvailBytes)
	}
}

func TestMeasureAvailReal(t *testing.T) {
	free, err := MeasureAvail(t.TempDir())
	if err != nil {
		t.Fatalf("temp dir must measure: %v", err)
	}
	if free == 0 {
		t.Fatalf("temp dir must report nonzero space")
	}
}
