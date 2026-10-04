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
}
