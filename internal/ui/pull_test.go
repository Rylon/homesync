package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Rylon/homesync/internal/git"
)

// pullModel builds an example model, on the pull screen, at the specified pull step.
func pullModel(step pullStep) Model {
	model := Model{screen: screenPull, repo: &git.Repo{Path: "/unused"}}
	model.branch = "main"
	model.pull.step = step
	return model
}

// Test that the various pullUpdateMessages triggers the correct step in the sequence.
func TestUpdatePullMsgMovesToTheRightStep(t *testing.T) {
	cases := []struct {
		name        string
		msg         tea.Msg
		wantStep    pullStep
		wantErr     bool
		wantRefresh bool
		wantPreview []git.Commit
	}{
		{
			name:     "a failed fetch goes back to the pull gate",
			msg:      pullIncomingMsg{err: errors.New("no origin")},
			wantStep: pullGate,
			wantErr:  true,
		},
		{
			name:        "a fetch with commits waiting shows the preview",
			msg:         pullIncomingMsg{before: "abc", commits: []git.Commit{{Hash: "abc1234"}}},
			wantStep:    pullPreview,
			wantPreview: []git.Commit{{Hash: "abc1234"}},
		},
		{
			name:     "a fetch with nothing waiting still shows the preview, but it says 'up to date'",
			msg:      pullIncomingMsg{before: "abc"},
			wantStep: pullPreview,
		},
		{
			name:     "conflicts don't attempt a refresh",
			msg:      pullReportMsg{conflicts: []string{"home/.zshrc"}},
			wantStep: pullConflicted,
		},
		{
			name:     "a paused rebase counts as conflicted even with no conflicted files",
			msg:      pullReportMsg{rebasing: true},
			wantStep: pullConflicted,
		},
		{
			name:        "a clean pull shows the report and refreshes",
			msg:         pullReportMsg{diffStat: "1 file changed"},
			wantStep:    pullReport,
			wantRefresh: true,
		},
		{
			name:        "a pull that failed for a reason other than conflicts reports the error",
			msg:         pullReportMsg{err: errors.New("linking failed")},
			wantStep:    pullReport,
			wantErr:     true,
			wantRefresh: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next, cmd := pullModel(pullFetching).updatePullMsg(testCase.msg)
			model := next.(Model)

			if model.pull.step != testCase.wantStep {
				t.Errorf("step = %d, want %d", model.pull.step, testCase.wantStep)
			}

			if (model.err != nil) != testCase.wantErr {
				t.Errorf("err = %v, want error: %v", model.err, testCase.wantErr)
			}

			if model.loading != testCase.wantRefresh || (cmd != nil) != testCase.wantRefresh {
				t.Errorf("loading = %v, cmd = %v, want refresh: %v", model.loading, cmd != nil, testCase.wantRefresh)
			}

			if testCase.wantPreview != nil && len(model.pull.incoming) != len(testCase.wantPreview) {
				t.Errorf("incoming = %+v, want %+v", model.pull.incoming, testCase.wantPreview)
			}
		})
	}
}
