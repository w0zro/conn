package main

import (
	"time"

	"github.com/w0zro/conn/internal/work"

	tea "charm.land/bubbletea/v2"
)

// The sources that speak on their own time rather than on the reading's
// beat, as the loop hears them: docker's feed, which answers its own
// events, and brew, asked on a slow beat of its own. work reads them;
// these turn what it read into words for the loop.

// dockerReadyMsg carries the feed, once it is running; nil where docker
// is not installed.
type dockerReadyMsg struct{ feed *work.DockerFeed }

// dockerMsg carries what docker says of its containers.
type dockerMsg struct{ work.DockerList }

// startDocker begins docker's feed.
func startDocker() tea.Msg { return dockerReadyMsg{feed: work.StartDocker()} }

// nextDocker waits for the feed's next word.
func nextDocker(f *work.DockerFeed) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg { return dockerMsg{f.Next()} }
}

// brewMsg carries what brew said of its services.
type brewMsg struct {
	services []work.BrewService
	err      error
}

// brewTickMsg is the beat on which brew is asked, while anything is
// declared.
type brewTickMsg struct{}

// nextBrew is the next beat.
func nextBrew() tea.Cmd {
	return tea.Tick(work.BrewBeat, func(time.Time) tea.Msg { return brewTickMsg{} })
}

// readBrew asks brew, off the loop.
func readBrew() tea.Msg {
	services, err := work.ReadBrew()
	return brewMsg{services: services, err: err}
}
