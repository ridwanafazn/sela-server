package privacy

// ContextPolicy reports which personal context categories may be used.
func ContextPolicy(mode string) (journals bool, memories bool) {
	switch mode {
	case ContextNone:
		return false, false
	case ContextJournals:
		return true, false
	case ContextJournalsAndMemories:
		return true, true
	default:
		return false, false
	}
}
