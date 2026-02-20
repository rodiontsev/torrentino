package shuffle

import (
	"math/rand"
	"sync"
	"time"
)

type Shuffle struct {
	mu sync.Mutex

	items  []string
	index  int
	random *rand.Rand

	history      []string
	historyLimit int
}

func CreateShuffle(items []string) *Shuffle {
	s := &Shuffle{
		items:        append([]string(nil), items...), //copy items
		random:       rand.New(rand.NewSource(time.Now().UnixNano())),
		historyLimit: 3,
	}

	s.reshuffle()

	return s
}

func (s *Shuffle) reshuffle() {
	s.random.Shuffle(len(s.items), func(i, j int) {
		s.items[i], s.items[j] = s.items[j], s.items[i]
	})
	s.index = 0

	//if fist N items were picked just before the reshuffle, move them to the end of the list
	var (
		items        []string
		seenRecently []string
	)

	for _, item := range s.items {
		if len(items) < s.historyLimit && s.seenRecently(item) {
			seenRecently = append(seenRecently, item)
		} else {
			items = append(items, item)
		}
	}

	s.items = append(items, seenRecently...)
}

func (s *Shuffle) remember(item string) {
	s.history = append(s.history, item)

	if len(s.history) > s.historyLimit {
		s.history = s.history[1:] //drop the oldest item
	}
}

func (s *Shuffle) seenRecently(item string) bool {
	for _, h := range s.history {
		if h == item {
			return true
		}
	}
	return false
}

func (s *Shuffle) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index >= len(s.items) {
		s.reshuffle()
	}

	item := s.items[s.index]

	s.remember(item)
	s.index++

	return item
}
