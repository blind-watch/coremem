package search

import (
	"regexp"
	"strings"
	"time"
)

type Candidate struct {
	Title      string
	Body       string
	Tags       []string
	Entities   []string
	FilePaths  []string
	Type       string
	Status     string
	RepoID     string
	UserID     string
	Importance float64
	CreatedAt  time.Time
}

var termRE = regexp.MustCompile(`[A-Za-z0-9_./-]+`)

func Terms(query string) []string {
	raw := termRE.FindAllString(strings.ToLower(query), -1)
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		if len(t) < 2 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func Score(c Candidate, query string, terms []string, repoID, userID string, now time.Time) float64 {
	hayTitle := strings.ToLower(c.Title)
	hayBody := strings.ToLower(c.Body)
	hayTags := strings.ToLower(strings.Join(c.Tags, " "))
	hayEntities := strings.ToLower(strings.Join(c.Entities, " "))
	hayFiles := strings.ToLower(strings.Join(c.FilePaths, " "))
	q := strings.ToLower(strings.TrimSpace(query))

	var score float64
	if q != "" && (strings.Contains(hayTitle, q) || strings.Contains(hayBody, q)) {
		score += 4
	}
	for _, term := range terms {
		switch {
		case strings.Contains(hayTitle, term):
			score += 2.5
		case strings.Contains(hayBody, term):
			score += 1.3
		}
		if strings.Contains(hayTags, term) || strings.Contains(hayEntities, term) || strings.Contains(hayFiles, term) {
			score += 1.5
		}
	}
	if repoID != "" && c.RepoID == repoID {
		score += 3
	}
	if userID != "" && c.UserID == userID {
		score += 2
	}
	if c.Status == "active" {
		score += 2
	}
	if strings.HasPrefix(c.Type, "core_") {
		score += 2
	}
	score += clamp(c.Importance, 0, 1) * 2
	if !c.CreatedAt.IsZero() {
		days := now.Sub(c.CreatedAt).Hours() / 24
		if days < 0 {
			days = 0
		}
		score += 0.75 / (1 + days/30)
	}
	return score
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
