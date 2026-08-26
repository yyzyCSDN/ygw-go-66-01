package chain

import (
	"fmt"

	"auditlog/internal/model"
)

// allowedTransitions 描述块状态的合法迁移关系。
var allowedTransitions = map[model.State]map[model.State]bool{
	model.StateActive: {
		model.StateArchived: true,
		model.StateExpired:  true,
	},
	model.StateArchived: {
		model.StateActive:  true,
		model.StateExpired: true,
	},
	model.StateExpired: {},
}

// TransitionAllowed 判断块状态迁移是否合法。
func TransitionAllowed(from, to model.State) error {
	targets, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("unknown source state %q", from)
	}
	if !targets[to] {
		return fmt.Errorf("state transition %q -> %q is not allowed", from, to)
	}
	return nil
}

// NextState 返回块按生命周期推进后的下一状态。
func NextState(current model.State) model.State {
	switch current {
	case model.StateActive:
		return model.StateArchived
	case model.StateArchived:
		return model.StateExpired
	default:
		return model.StateExpired
	}
}

// StateLabel 返回状态的中文展示名。
func StateLabel(state model.State) string {
	switch state {
	case model.StateActive:
		return "活跃"
	case model.StateArchived:
		return "已归档"
	case model.StateExpired:
		return "已过期"
	default:
		return string(state)
	}
}
