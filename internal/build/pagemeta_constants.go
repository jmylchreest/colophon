package build

// Reading-time tuning. Average English silent reading is ~200-240 wpm; 185 is a slightly
// slower, more honest pace for longer/technical posts. secondsPerVisual is added per image
// or diagram, which take time to study, so visuals lengthen the estimate rather than be ignored.
const (
	wordsPerMinute   = 185
	secondsPerVisual = 30
)
