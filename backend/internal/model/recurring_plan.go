package model

import (
	"time"

	"github.com/aasplit/aasplit/internal/constants"
)

// RecurringPlan 周期账单计划实体：每月固定执行日自动生成一笔消费记录。
type RecurringPlan struct {
	ID              uint                          `gorm:"primaryKey" json:"id"`
	GroupID         uint                          `gorm:"index;not null;index:idx_plan_group_name,unique,where:status <> 'removed',priority:1" json:"group_id"`
	Name            string                        `gorm:"size:128;not null;index:idx_plan_group_name,unique,where:status <> 'removed',priority:2" json:"name"`
	Amount          float64                       `gorm:"type:double precision;not null" json:"amount"`
	Category        constants.ExpenseCategory     `gorm:"size:32;not null" json:"category"`
	PayerID         uint                          `gorm:"index;not null" json:"payer_id"`
	SplitType       constants.SplitType           `gorm:"size:16;not null" json:"split_type"`
	DayOfMonth      int                           `gorm:"not null" json:"day_of_month"`
	Status          constants.RecurringPlanStatus `gorm:"size:16;not null;default:active;index" json:"status"`
	LastRunPeriod   string                        `gorm:"size:7" json:"last_run_period"`
	LastExpenseID   *uint                         `json:"last_expense_id"`
	LastGeneratedAt *time.Time                    `json:"last_generated_at"`
	CreatedBy       uint                          `json:"created_by"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`

	// 关联（不持久化外键约束，仅用于查询填充）
	Payer       *User                `gorm:"foreignKey:PayerID" json:"payer,omitempty"`
	Shares      []RecurringPlanShare `gorm:"foreignKey:PlanID" json:"shares,omitempty"`
	LastExpense *Expense             `gorm:"foreignKey:LastExpenseID" json:"last_expense,omitempty"`
}

// TableName 指定表名。
func (RecurringPlan) TableName() string { return "recurring_plans" }

// IsActive 判断计划是否启用中。
func (p *RecurringPlan) IsActive() bool { return p.Status == constants.PlanActive }

// IsPaused 判断计划是否已暂停。
func (p *RecurringPlan) IsPaused() bool { return p.Status == constants.PlanPaused }

// IsRemoved 判断计划是否已移除。
func (p *RecurringPlan) IsRemoved() bool { return p.Status == constants.PlanRemoved }

// PayerName 返回付款人昵称（关联未加载时回退为空字符串）。
func (p *RecurringPlan) PayerName() string {
	if p.Payer != nil {
		return p.Payer.Nickname
	}
	return ""
}

// RecurringPlanShare 周期账单计划参与人明细：生成消费时作为分摊输入。
type RecurringPlanShare struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PlanID    uint      `gorm:"index:idx_plan_share_plan_user,unique;not null" json:"plan_id"`
	UserID    uint      `gorm:"index:idx_plan_share_plan_user,unique;not null" json:"user_id"`
	Ratio     float64   `gorm:"type:double precision;default:0" json:"ratio"`
	Amount    float64   `gorm:"type:double precision;default:0" json:"amount"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName 指定表名。
func (RecurringPlanShare) TableName() string { return "recurring_plan_shares" }

// RecurringPlanRun 周期账单计划生成记录：每个执行周期（YYYY-MM）最多一笔，唯一索引兜底幂等。
type RecurringPlanRun struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PlanID      uint      `gorm:"index:idx_plan_run_plan_period,unique,priority:1;not null" json:"plan_id"`
	Period      string    `gorm:"size:7;not null;index:idx_plan_run_plan_period,unique,priority:2" json:"period"`
	ExpenseID   uint      `gorm:"not null" json:"expense_id"`
	GeneratedAt time.Time `json:"generated_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 指定表名。
func (RecurringPlanRun) TableName() string { return "recurring_plan_runs" }

// daysInMonth 返回某年某月的天数（月末没有对应日期时落到当月最后一天的基础）。
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// EffectiveDate 计算某周期（年月）的实际执行日：day_of_month 超过当月天数时落到当月最后一天。
func EffectiveDate(year int, month time.Month, dayOfMonth int) time.Time {
	day := dayOfMonth
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

// PeriodOf 返回时间所属的月度执行周期（YYYY-MM）。
func PeriodOf(t time.Time) string {
	return t.Format("2006-01")
}

// NextRunDate 计算下次执行日：今天之后（不含今天）最近的一个有效执行日。
// 调用方在生成完所有到期周期后再计算，因此结果总是未来的执行日。
func NextRunDate(dayOfMonth int, now time.Time) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	year, month := today.Year(), today.Month()
	for i := 0; i < 25; i++ {
		d := EffectiveDate(year, month, dayOfMonth)
		if d.After(today) {
			return d
		}
		month++
		if month > time.December {
			month = time.January
			year++
		}
	}
	return today
}

// DueDates 计算截至 now 全部到期的执行日（升序）：
// 执行日不早于计划创建日、不晚于今天，且对应周期尚未生成过。
func DueDates(plan *RecurringPlan, now time.Time, ran map[string]bool) []time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	created := time.Date(plan.CreatedAt.Year(), plan.CreatedAt.Month(), plan.CreatedAt.Day(), 0, 0, 0, 0, time.Local)
	year, month := created.Year(), created.Month()
	var due []time.Time
	for i := 0; i < 1200; i++ {
		d := EffectiveDate(year, month, plan.DayOfMonth)
		if d.After(today) {
			break
		}
		if !d.Before(created) && !ran[PeriodOf(d)] {
			due = append(due, d)
		}
		month++
		if month > time.December {
			month = time.January
			year++
		}
	}
	return due
}
