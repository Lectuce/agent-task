package cron

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CronJob struct {
	ID        string
	Cron      string
	Prompt    string
	Recurring bool
	Durable   bool
}

type Manager struct {
	mu          sync.Mutex
	jobs        map[string]*CronJob
	queue       []*CronJob
	lastFired   map[string]string
	durablePath string
}

func NewManager(durablePath string) *Manager {
	return &Manager{
		jobs:        make(map[string]*CronJob),
		queue:       make([]*CronJob, 0),
		lastFired:   make(map[string]string, 0),
		durablePath: durablePath,
	}

}

func cronFieldMatches(field string, value int) bool {

	if field == "*" {
		return true
	}

	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil {
			return false
		}

		return step > 0 && value%step == 0
	}

	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			if cronFieldMatches(strings.TrimSpace(part), value) {
				return true
			}
		}

		return false
	}

	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)
		if len(parts) != 2 {
			return false
		}

		loww, err1 := strconv.Atoi(parts[0])
		highgh, err2 := strconv.Atoi(parts[1])

		if err1 != nil || err2 != nil {
			return false
		}

		return value >= loww && value <= highgh
	}

	n, err := strconv.Atoi(field)
	if err != nil {
		return false
	}

	return value == n
}

func CronMatches(expr string, now time.Time) bool {

	fields := strings.Fields(expr)

	if len(fields) != 5 {
		return false
	}

	minute := fields[0]
	hour := fields[1]
	dom := fields[2]
	month := fields[3]
	dow := fields[4]

	m := cronFieldMatches(minute, now.Minute())

	h := cronFieldMatches(hour, now.Hour())

	domOK := cronFieldMatches(dom, now.Day())

	monthOK := cronFieldMatches(month, int(now.Month()))

	dowOK := cronFieldMatches(dow, int(now.Weekday()))

	if !(m && h && monthOK) {
		return false
	}

	domUnconstrained := dom == "*"
	dowUnconstrained := dow == "*"

	if domUnconstrained && dowUnconstrained {
		return true
	}

	if domUnconstrained {
		return dowOK
	}

	if dowUnconstrained {
		return domOK
	}

	return domOK || dowOK
}

func validateCronField(field string, loww int, highgh int) error {

	if field == "*" {
		return nil
	}

	if strings.HasPrefix(field, "*/") {
		stepStr := field[2:]

		step, err := strconv.Atoi(stepStr)
		if err != nil {
			return fmt.Errorf("invalid step: %s", field)
		}

		if step <= 0 {
			return fmt.Errorf("step must be > 0: %s", field)
		}

		return nil
	}

	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			err := validateCronField(strings.TrimSpace(part), loww, highgh)
			if err != nil {
				return err
			}
		}

		return nil
	}

	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)

		if len(parts) != 2 {
			return fmt.Errorf("invalid range: %s", field)
		}

		a, err1 := strconv.Atoi(parts[0])
		b, err2 := strconv.Atoi(parts[1])

		if err1 != nil || err2 != nil {
			return fmt.Errorf(
				"invalid range: %s",
				field,
			)
		}

		if a < loww || a > highgh || b < loww || b > highgh {

			return fmt.Errorf("range %s out of bounds [%d-%d]", field, loww, highgh)
		}

		if a > b {
			return fmt.Errorf("range start > end: %s", field)
		}

		return nil
	}

	value, err := strconv.Atoi(field)
	if err != nil {
		return fmt.Errorf("invalid field: %s", field)
	}

	if value < loww || value > highgh {
		return fmt.Errorf("value %d out of bounds [%d-%d]", value, loww, highgh)
	}

	return nil
}

func ValidateCron(expr string) error {
	fields := strings.Fields(expr)

	if len(fields) != 5 {
		return fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	bounds := [][2]int{
		{0, 59},
		{0, 23},
		{1, 31},
		{1, 12},
		{0, 6},
	}

	names := []string{"minute", "hour", "day-of-month", "month", "day-of-week"}

	for i, field := range fields {
		err := validateCronField(field, bounds[i][0], bounds[i][1])
		if err != nil {

			return fmt.Errorf("%s: %w", names[i], err)
		}
	}

	return nil
}
