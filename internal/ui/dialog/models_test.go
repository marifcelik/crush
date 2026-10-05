package dialog

import (
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestModelTypeSlot(t *testing.T) {
	t.Parallel()

	require.Equal(t, config.PlanModeSlot(""), ModelTypeLarge.Slot())
	require.Equal(t, config.PlanModeSlot(""), ModelTypeSmall.Slot())
	require.Equal(t, config.PlanModeSlotPlan, ModelTypePlan.Slot())
}

func TestModelTypeCycleStaysInRange(t *testing.T) {
	t.Parallel()

	require.Equal(t, ModelTypeLarge, ModelType((int(ModelTypePlan)+1)%numModelTypes))
}
