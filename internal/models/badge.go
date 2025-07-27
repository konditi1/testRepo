package models

// Badge represents an achievement badge that users can earn
// by reaching certain milestones or completing specific actions.
type Badge struct {
	ID            int     `json:"id" db:"id"`
	Name          string  `json:"name" db:"name"`
	Description   string  `json:"description" db:"description"`
	Icon          string  `json:"icon" db:"icon"`
	Color         string  `json:"color" db:"color"`
	CriteriaType  string  `json:"criteria_type" db:"criteria_type"`
	CriteriaValue int     `json:"criteria_value" db:"criteria_value"`
	IsActive      bool    `json:"is_active" db:"is_active"`
	CreatedAt     string  `json:"created_at" db:"created_at"`
	UpdatedAt     *string `json:"updated_at,omitempty" db:"updated_at"`
}
