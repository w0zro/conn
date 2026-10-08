package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/w0zro/conn/internal/work/brew"
	"github.com/w0zro/conn/internal/work/docker"
)

// The sources that speak on their own time rather than on the reading's
// beat, as the loop hears them: docker's feed, which answers its own
// events, and brew, asked on a slow beat of its own. work reads them;
// these turn what it read into words for the loop.

// dockerReadyMsg carries the feed, once it is running; nil where docker
// is not installed.
type dockerReadyMsg struct{ feed *docker.Feed }

// dockerMsg carries what docker says of its containers.
type dockerMsg struct{ docker.List }

// startDocker begins docker's feed.
func startDocker() tea.Msg { return dockerReadyMsg{feed: docker.Start()} }

// nextDocker waits for the feed's next word.
func nextDocker(f *docker.Feed) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg { return dockerMsg{f.Next()} }
}

// brewMsg carries what brew said of its services.
type brewMsg struct {
	services []brew.Service
	err      error
}

// brewTickMsg is the beat on which brew is asked, while anything is
// declared.
type brewTickMsg struct{}

// nextBrew is the next beat.
func nextBrew() tea.Cmd {
	return tea.Tick(brew.Beat, func(time.Time) tea.Msg { return brewTickMsg{} })
}

// readBrew asks brew, off the loop.
func readBrew() tea.Msg {
	services, err := brew.Read()
	return brewMsg{services: services, err: err}
}
