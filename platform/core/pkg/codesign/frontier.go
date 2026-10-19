package codesign

// MAP-Elites (T,Q) grid: min-TQ per log-bucket cell. Read-only serving
// mirror of the Python frontier; the evaluator remains Python-owned.

type Entry struct {
	Toffoli int   `json:"toffoli"`
	Qubits  int   `json:"qubits"`
	TQ      int64 `json:"tq"`
}

type Frontier struct {
	TBuckets int
	QBuckets int
	Kappa    int
	cells    map[[2]int]Entry
}

func NewFrontier(tBuckets, qBuckets, kappa int) *Frontier {
	if tBuckets <= 0 {
		tBuckets = 12
	}
	if qBuckets <= 0 {
		qBuckets = 12
	}
	if kappa < KappaMin || kappa > KappaMax {
		kappa = KappaDefault
	}
	return &Frontier{TBuckets: tBuckets, QBuckets: qBuckets, Kappa: kappa, cells: map[[2]int]Entry{}}
}

func bucket(v, n int) int {
	if v <= 0 {
		return 0
	}
	b := 0
	for x := v; x > 0; x >>= 1 {
		b++
	}
	if b >= n {
		return n - 1
	}
	return b
}

func (f *Frontier) cell(t, q int) [2]int {
	return [2]int{bucket(t, f.TBuckets), bucket(q, f.QBuckets)}
}

// Offer keeps min-TQ per cell. Returns true when the cell was created or improved.
func (f *Frontier) Offer(toffoli, qubits int) (bool, error) {
	tq, err := Cost(toffoli, qubits, f.Kappa)
	if err != nil {
		return false, err
	}
	c := f.cell(toffoli, qubits)
	cur, ok := f.cells[c]
	if !ok || tq < cur.TQ {
		f.cells[c] = Entry{Toffoli: toffoli, Qubits: qubits, TQ: tq}
		return true, nil
	}
	return false, nil
}

func (f *Frontier) Best() *Entry {
	var best *Entry
	for _, e := range f.cells {
		ec := e
		if best == nil || ec.TQ < best.TQ {
			best = &ec
		}
	}
	return best
}

func (f *Frontier) Entries() []Entry {
	out := make([]Entry, 0, len(f.cells))
	for _, e := range f.cells {
		out = append(out, e)
	}
	// insertion order is nondeterministic; callers sort by TQ when needed.
	return out
}
