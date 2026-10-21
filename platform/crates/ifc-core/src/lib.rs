//! Generic information-flow control over reader-set confidentiality labels.
//! Implements Buzz's can_flow_to reader-set lattice in Rust for Kubona.

use std::collections::BTreeSet;

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum ReaderSet<Principal: Ord> {
    Everyone,
    Only(BTreeSet<Principal>),
}

impl<Principal: Ord + Clone> ReaderSet<Principal> {
    pub fn can_flow_to(&self, destination: &Self) -> bool {
        match (self, destination) {
            (Self::Everyone, _) => true,
            (Self::Only(_), Self::Everyone) => false,
            (Self::Only(source), Self::Only(destination)) => destination.is_subset(source),
        }
    }

    pub fn join(&self, other: &Self) -> Self {
        match (self, other) {
            (Self::Everyone, value) | (value, Self::Everyone) => value.clone(),
            (Self::Only(left), Self::Only(right)) => {
                Self::Only(left.intersection(right).cloned().collect())
            }
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ConfidentialityLabel<Universe: Ord, Principal: Ord> {
    pub universe: Universe,
    pub readers: ReaderSet<Principal>,
}

impl<Universe: Ord + Clone, Principal: Ord + Clone> ConfidentialityLabel<Universe, Principal> {
    pub fn public(universe: Universe) -> Self {
        Self {
            universe,
            readers: ReaderSet::Everyone,
        }
    }

    pub fn restricted(universe: Universe, readers: BTreeSet<Principal>) -> Self {
        Self {
            universe,
            readers: ReaderSet::Only(readers),
        }
    }

    pub fn can_flow_to(&self, destination: &Self) -> bool {
        if self.universe != destination.universe {
            return false;
        }
        self.readers.can_flow_to(&destination.readers)
    }

    pub fn join(&self, other: &Self) -> Option<Self> {
        if self.universe != other.universe {
            return None;
        }
        Some(Self {
            universe: self.universe.clone(),
            readers: self.readers.join(&other.readers),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_public_flows_anywhere() {
        let pub_label: ConfidentialityLabel<&str, &str> = ConfidentialityLabel::public("lab");
        let mut restricted_set = BTreeSet::new();
        restricted_set.insert("alice");
        let priv_label = ConfidentialityLabel::restricted("lab", restricted_set);

        assert!(pub_label.can_flow_to(&priv_label));
        assert!(pub_label.can_flow_to(&pub_label));
    }

    #[test]
    fn test_private_cannot_flow_to_public() {
        let pub_label: ConfidentialityLabel<&str, &str> = ConfidentialityLabel::public("lab");
        let mut restricted_set = BTreeSet::new();
        restricted_set.insert("alice");
        let priv_label = ConfidentialityLabel::restricted("lab", restricted_set);

        assert!(!priv_label.can_flow_to(&pub_label));
    }

    #[test]
    fn test_restricted_narrowing_is_safe() {
        let mut set_ab = BTreeSet::new();
        set_ab.insert("alice");
        set_ab.insert("bob");
        let label_ab = ConfidentialityLabel::restricted("lab", set_ab);

        let mut set_a = BTreeSet::new();
        set_a.insert("alice");
        let label_a = ConfidentialityLabel::restricted("lab", set_a);

        // Information readable by Alice and Bob can flow to a context read only by Alice
        assert!(label_ab.can_flow_to(&label_a));
        // But not the other way around (cannot widen readers to Bob)
        assert!(!label_a.can_flow_to(&label_ab));
    }

    #[test]
    fn test_cross_universe_blocked() {
        let label1: ConfidentialityLabel<&str, &str> = ConfidentialityLabel::public("lab-1");
        let label2: ConfidentialityLabel<&str, &str> = ConfidentialityLabel::public("lab-2");
        assert!(!label1.can_flow_to(&label2));
    }

    fn readers<'a>(names: &[&'a str]) -> BTreeSet<&'a str> {
        names.iter().copied().collect()
    }

    // T-14: transitivity — the property the 4 original tests never pinned.
    // A broken chain must stay broken: {a} -> {a,b} is false, so no path
    // through {a,b} can launder {a}-readable data to {a,b} readers.
    #[test]
    fn test_flow_is_transitive() {
        let ab = ReaderSet::Only(readers(&["alice", "bob"]));
        let a = ReaderSet::Only(readers(&["alice"]));
        assert!(ab.can_flow_to(&a));
        assert!(a.can_flow_to(&a));
        // Negative: widening is denied, so transitivity cannot be abused
        // to reach a wider context in two hops.
        assert!(!a.can_flow_to(&ab));
    }

    // T-14: reflexivity for restricted sets + antisymmetry of the order.
    #[test]
    fn test_flow_is_reflexive_and_antisymmetric() {
        let a = ReaderSet::Only(readers(&["alice"]));
        let b = ReaderSet::Only(readers(&["bob"]));
        assert!(a.can_flow_to(&a));
        assert!(b.can_flow_to(&b));
        // Antisymmetry on labels: mutual flow implies identical labels.
        let la1 = ConfidentialityLabel::restricted("lab", readers(&["alice"]));
        let la2 = ConfidentialityLabel::restricted("lab", readers(&["alice"]));
        assert!(la1.can_flow_to(&la2) && la2.can_flow_to(&la1));
        assert_eq!(la1, la2);
        // Distinct reader sets never flow both ways.
        let lb = ConfidentialityLabel::restricted("lab", readers(&["bob"]));
        assert!(!(la1.can_flow_to(&lb) && lb.can_flow_to(&la1)));
    }

    // T-14: the empty reader set, pinned as INTENDED (asymmetry decision,
    // audit 2026-10-26). Narrowing any context into Only({}) is safe —
    // data flows to a context nobody reads (availability, never a leak).
    // Only({}) as a SOURCE flows only to itself. This differs from Go's
    // `deny-all` (`lattice.go:100-109`) on purpose: Go answers "can
    // principal P read label L" (readability — empty reads as false),
    // Rust answers "can context A flow to context B" (narrowing into
    // empty is safe). If either side changes its operation, revisit.
    #[test]
    fn test_empty_reader_set_semantics() {
        let empty: ReaderSet<&str> = ReaderSet::Only(readers(&[]));
        let a = ReaderSet::Only(readers(&["alice"]));
        let everyone: ReaderSet<&str> = ReaderSet::Everyone;
        // Narrowing into empty is safe (data disappears, never leaks).
        assert!(a.can_flow_to(&empty));
        assert!(everyone.can_flow_to(&empty));
        // Empty as source flows only to itself — never widens.
        assert!(empty.can_flow_to(&empty));
        assert!(!empty.can_flow_to(&a));
        assert!(!empty.can_flow_to(&everyone));
    }

    // T-14: join is a least upper bound. Tenant-mix case mirrors Go's
    // `tenant/a + tenant/b -> deny-all` (lattice_test.go:49-51): the join
    // is empty, and per test_empty_reader_set_semantics the empty join
    // flows nowhere except itself — the same fail-closed outcome.
    #[test]
    fn test_join_is_least_upper_bound() {
        let a = ReaderSet::Only(readers(&["alice"]));
        let b = ReaderSet::Only(readers(&["bob"]));
        let ab = ReaderSet::Only(readers(&["alice", "bob"]));
        // Commutativity + idempotence.
        assert_eq!(a.join(&b), b.join(&a));
        assert_eq!(a.join(&a), a);
        // Upper bound: both sides flow into the join.
        let jab = ab.join(&a);
        assert_eq!(jab, a);
        assert!(ab.can_flow_to(&jab) && a.can_flow_to(&jab));
        // Leastness: the join flows to z iff both sides flow to z.
        let z = ReaderSet::Only(readers(&["alice"]));
        assert!(jab.can_flow_to(&z) == (ab.can_flow_to(&z) && a.can_flow_to(&z)));
        // Tenant mix: disjoint tenants join to empty, which flows nowhere.
        let mixed = a.join(&b);
        assert_eq!(mixed, ReaderSet::Only(readers(&[])));
        assert!(!mixed.can_flow_to(&a));
        assert!(!mixed.can_flow_to(&b));
    }

    // T-14: label-level join — universe gate + reader LUB together.
    #[test]
    fn test_label_join() {
        let la = ConfidentialityLabel::restricted("lab", readers(&["alice", "bob"]));
        let lb = ConfidentialityLabel::restricted("lab", readers(&["bob"]));
        let joined = la.join(&lb).expect("same universe must join");
        assert_eq!(joined.readers, ReaderSet::Only(readers(&["bob"])));
        // Cross-universe join is None (fail-closed, mirrors can_flow_to).
        let other = ConfidentialityLabel::public("other-lab");
        assert!(la.join(&other).is_none());
    }
}
