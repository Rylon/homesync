package link

import "testing"

func TestSummariseCountsEachKind(t *testing.T) {
	got := Summarise([]Action{
		{Kind: Identical},
		{Kind: Identical},
		{Kind: Create},
		{Kind: Conflict},
		{Kind: SymlinkConflict},
		{Kind: Refused},
	})

	if got.Identical != 2 {
		t.Errorf("Identical = %d, want 2", got.Identical)
	}
	if got.Missing != 1 {
		t.Errorf("Missing = %d, want 1", got.Missing)
	}
	// Both conflict kinds are reported together.
	if got.Conflicts != 2 {
		t.Errorf("Conflicts = %d, want 2", got.Conflicts)
	}
	if got.Refused != 1 {
		t.Errorf("Refused = %d, want 1", got.Refused)
	}
}

func TestSummariseOnFullyLinkedCastleReportsNoProblems(t *testing.T) {
	got := Summarise([]Action{{Kind: Identical}, {Kind: Identical}})

	if got.Problems() != 0 {
		t.Errorf("Problems = %d, want 0", got.Problems())
	}
}

func TestSummaryProblemsCountsMissingConflictsAndRefused(t *testing.T) {
	got := Summarise([]Action{
		{Kind: Identical},
		{Kind: Create},
		{Kind: Conflict},
		{Kind: Refused},
	})

	if got.Problems() != 3 {
		t.Errorf("Problems = %d, want 3", got.Problems())
	}
}

func TestProblemsDropsIdenticalLinksAndKeepsOrder(t *testing.T) {
	got := Problems([]Action{
		{Kind: Identical, Rel: ".zshrc"},
		{Kind: Conflict, Rel: ".vimrc"},
		{Kind: Identical, Rel: ".inputrc"},
		{Kind: Create, Rel: ".example.toml"},
	})

	if len(got) != 2 || got[0].Rel != ".vimrc" || got[1].Rel != ".example.toml" {
		t.Errorf("Problems = %+v, want .vimrc then .example.toml", got)
	}
}
