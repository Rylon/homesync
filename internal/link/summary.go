package link

// Summary tallies a plan by outcome, so the UI can show counts without walking the plan itself.
type Summary struct {
	Identical int
	Missing   int
	Conflicts int
	Refused   int
}

// Problems counts the actions that need a human decision. Missing links are included because
// `Apply` only creates them on request.
func (summary Summary) Problems() int {
	return summary.Missing + summary.Conflicts + summary.Refused
}

// Summarise counts a plan by Kind. Both conflict kinds are counted together, because the
// resolution is the same for each: overwrite the destination, one file at a time.
func Summarise(actions []Action) Summary {
	var summary Summary
	for _, action := range actions {
		switch action.Kind {
		case Identical:
			summary.Identical++
		case Create:
			summary.Missing++
		case Conflict, SymlinkConflict:
			summary.Conflicts++
		case Refused:
			summary.Refused++
		}
	}
	return summary
}

// Problems returns the actions that need a human decision, in plan order. Identical links are
// dropped, as there is nothing to do for them.
func Problems(actions []Action) []Action {
	var problems []Action
	for _, action := range actions {
		if action.Kind != Identical {
			problems = append(problems, action)
		}
	}
	return problems
}
