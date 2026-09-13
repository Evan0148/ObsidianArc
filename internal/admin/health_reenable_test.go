package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/model"
)

// A reset re-enables the models the checker switched off on its own. A write
// that fails must be visible: the operator asked for a clean start, and a
// model left disabled is indistinguishable from one that was already healthy
// if all the response carries is a count of the ones that worked.
func TestAModelThatCouldNotBeReenabledIsNamed(t *testing.T) {
	const (
		offOne = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		onOne  = "01ARZ3NDEKTSV4RRFFQ69G5FAA"
		offTwo = "01ARZ3NDEKTSV4RRFFQ69G5FAB"
	)
	rows := []model.Model{
		{ID: offOne, AutoDisabled: true},
		{ID: onOne, AutoDisabled: false},
		{ID: offTwo, AutoDisabled: true},
	}

	var asked []string
	update := func(_ context.Context, id string, in model.Update) (model.Model, error) {
		asked = append(asked, id)
		// Every attempt must clear the checker's switch as well as turning the
		// model back on; one without the other leaves it disabled in the list.
		if in.Enabled == nil || !*in.Enabled {
			t.Errorf("update for %s did not enable it: %+v", id, in)
		}
		if in.AutoDisabled == nil || *in.AutoDisabled {
			t.Errorf("update for %s left auto_disabled set: %+v", id, in)
		}
		if id == offTwo {
			return model.Model{}, errors.New("write refused")
		}
		return model.Model{ID: id}, nil
	}

	reenabled, refused := reenableAutoDisabled(context.Background(), rows, update)

	if reenabled != 1 {
		t.Errorf("reenabled = %d, want only the write that succeeded", reenabled)
	}
	if len(refused) != 1 || refused[0] != offTwo {
		t.Errorf("refused = %v, want just %s", refused, offTwo)
	}
	// A model that was never switched off is not written to at all.
	if len(asked) != 2 {
		t.Errorf("wrote to %v, want only the two auto-disabled models", asked)
	}
}

// The ordinary case must stay silent, so the response does not carry a field
// that is always present and always empty.
func TestAResetThatReenablesEverythingNamesNothing(t *testing.T) {
	rows := []model.Model{{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", AutoDisabled: true}}
	update := func(_ context.Context, id string, _ model.Update) (model.Model, error) {
		return model.Model{ID: id}, nil
	}

	reenabled, refused := reenableAutoDisabled(context.Background(), rows, update)
	if reenabled != 1 || len(refused) != 0 {
		t.Errorf("reenabled = %d, refused = %v, want 1 and nothing refused", reenabled, refused)
	}
}
