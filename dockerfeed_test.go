package main

import (
	"strings"
	"testing"

	"github.com/w0zro/conn/internal/work"
)

// The panel holds docker's word and merges it into the next reading, and
// says so under the rows while docker is quiet.
func TestAStalledDockerIsSaidUnderTheRows(t *testing.T) {
	m := plainModel()
	m.view, m.width, m.height = viewProcesses, 48, 30
	next, _ := m.Update(work.DockerMsg{Containers: []work.Container{{ID: "abc", Service: "web", State: "running"}}, Stalled: true})
	m = next.(model)
	if !m.dockerStalled || len(m.containers) != 1 {
		t.Fatalf("the panel did not take docker's word: stalled %v, %d containers", m.dockerStalled, len(m.containers))
	}
	if text := texts(drawProcesses(m.processesReport(), m.cursor, 48, 30, Plain)); !strings.Contains(text, "AS LAST SEEN") {
		t.Errorf("the view does not say docker is quiet:\n%s", text)
	}
	// And it stops saying so once docker answers again.
	next, _ = m.Update(work.DockerMsg{Containers: []work.Container{{ID: "abc", Service: "web", State: "running"}}})
	m = next.(model)
	if text := texts(drawProcesses(m.processesReport(), m.cursor, 48, 30, Plain)); strings.Contains(text, "AS LAST SEEN") {
		t.Errorf("the view still says docker is quiet:\n%s", text)
	}
}
