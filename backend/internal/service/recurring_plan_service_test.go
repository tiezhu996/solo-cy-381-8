package service

import (
	"testing"
	"time"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
	"gorm.io/gorm"
)

// newPlanServiceFixture 构造带群组与三名成员的服务夹具（含可注入时钟）。
func newPlanServiceFixture(t *testing.T) (*gorm.DB, *RecurringPlanService, *ExpenseService, *GroupService, uint, uint, uint, uint) {
	t.Helper()
	db := newTestDB(t)
	userRepo := repository.NewUserRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	memberRepo := repository.NewGroupMemberRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	auditSvc := NewAuditService(auditRepo, newTestLogger())
	logger := newTestLogger()

	e1 := "alice@test.com"
	e2 := "bob@test.com"
	u1 := &model.User{Username: "alice", PasswordHash: "x", Nickname: "Alice", Email: &e1, Role: constants.RoleUser}
	u2 := &model.User{Username: "bob", PasswordHash: "x", Nickname: "Bob", Email: &e2, Role: constants.RoleUser}
	u3 := &model.User{Username: "carol", PasswordHash: "x", Nickname: "Carol", Role: constants.RoleUser}
	if err := userRepo.Create(u1); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := userRepo.Create(u2); err != nil {
		t.Fatalf("create u2: %v", err)
	}
	if err := userRepo.Create(u3); err != nil {
		t.Fatalf("create u3: %v", err)
	}
	groupSvc := NewGroupService(db, groupRepo, memberRepo, userRepo, auditSvc, logger)
	group, err := groupSvc.Create(u1.ID, &dto.CreateGroupReq{Name: "合租生活", Description: "测试"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := groupSvc.InviteMember(u1.ID, group.ID, "bob"); err != nil {
		t.Fatalf("invite bob: %v", err)
	}
	if err := groupSvc.InviteMember(u1.ID, group.ID, "carol"); err != nil {
		t.Fatalf("invite carol: %v", err)
	}
	expenseSvc := NewExpenseService(db, repository.NewExpenseRepository(db), memberRepo, groupRepo, userRepo, auditSvc, logger)
	planSvc := NewRecurringPlanService(db, repository.NewRecurringPlanRepository(db), groupRepo, memberRepo, expenseSvc, auditSvc, logger)
	return db, planSvc, expenseSvc, groupSvc, group.ID, u1.ID, u2.ID, u3.ID
}

// countGroupExpenses 统计群组有效消费笔数。
func countGroupExpenses(t *testing.T, db *gorm.DB, groupID uint) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.Expense{}).Where("group_id = ? AND status = ?", groupID, constants.ExpenseActive).Count(&n).Error; err != nil {
		t.Fatalf("count expenses: %v", err)
	}
	return n
}

// equalPlanReq 构造三人均摊的计划请求。
func equalPlanReq(groupID, payerID, a, b, c uint, name string, day int) *dto.CreateRecurringPlanReq {
	return &dto.CreateRecurringPlanReq{
		GroupID: groupID, Name: name, Amount: 300, Category: "lodging",
		PayerID: payerID, SplitType: "equal", DayOfMonth: day,
		Shares: []dto.ShareInput{{UserID: a}, {UserID: b}, {UserID: c}},
	}
}

func TestEffectiveDateMonthEndClamp(t *testing.T) {
	tests := []struct {
		name       string
		year       int
		month      time.Month
		dayOfMonth int
		wantDay    int
	}{
		{name: "normal day", year: 2026, month: 9, dayOfMonth: 15, wantDay: 15},
		{name: "31 falls to Sep 30", year: 2026, month: 9, dayOfMonth: 31, wantDay: 30},
		{name: "31 falls to Feb 28", year: 2026, month: 2, dayOfMonth: 31, wantDay: 28},
		{name: "31 falls to Feb 29 leap", year: 2024, month: 2, dayOfMonth: 31, wantDay: 29},
		{name: "30 falls to Feb 28", year: 2026, month: 2, dayOfMonth: 30, wantDay: 28},
		{name: "31 stays in Jan", year: 2026, month: 1, dayOfMonth: 31, wantDay: 31},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.EffectiveDate(tt.year, tt.month, tt.dayOfMonth)
			if got.Day() != tt.wantDay || got.Month() != tt.month || got.Year() != tt.year {
				t.Fatalf("EffectiveDate(%d,%d,%d) = %v, want day %d", tt.year, tt.month, tt.dayOfMonth, got, tt.wantDay)
			}
		})
	}
}

func TestNextRunDate(t *testing.T) {
	tests := []struct {
		name       string
		dayOfMonth int
		now        time.Time
		want       time.Time
	}{
		{name: "later this month", dayOfMonth: 15, now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.Local), want: time.Date(2026, 9, 15, 0, 0, 0, 0, time.Local)},
		{name: "today rolls to next month", dayOfMonth: 15, now: time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local), want: time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local)},
		{name: "passed this month", dayOfMonth: 10, now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.Local), want: time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local)},
		{name: "31 clamps to Sep 30 then Oct 31", dayOfMonth: 31, now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.Local), want: time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)},
		{name: "31 from Feb clamps to Mar 31", dayOfMonth: 31, now: time.Date(2026, 2, 28, 10, 0, 0, 0, time.Local), want: time.Date(2026, 3, 31, 0, 0, 0, 0, time.Local)},
		{name: "year rollover", dayOfMonth: 5, now: time.Date(2026, 12, 31, 10, 0, 0, 0, time.Local), want: time.Date(2027, 1, 5, 0, 0, 0, 0, time.Local)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.NextRunDate(tt.dayOfMonth, tt.now); !got.Equal(tt.want) {
				t.Fatalf("NextRunDate(%d, %v) = %v, want %v", tt.dayOfMonth, tt.now, got, tt.want)
			}
		})
	}
}

func TestDueDates(t *testing.T) {
	created := time.Date(2026, 7, 20, 12, 0, 0, 0, time.Local)
	plan := &model.RecurringPlan{DayOfMonth: 15, CreatedAt: created}
	tests := []struct {
		name string
		now  time.Time
		ran  map[string]bool
		want []string // 期望到期执行日 YYYY-MM-DD
	}{
		{name: "not due before first run day", now: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local), want: nil},
		{name: "due on run day", now: time.Date(2026, 8, 15, 0, 0, 0, 0, time.Local), want: []string{"2026-08-15"}},
		{name: "catch up multiple periods", now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), want: []string{"2026-08-15", "2026-09-15"}},
		{name: "skip already ran periods", now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), ran: map[string]bool{"2026-08": true}, want: []string{"2026-09-15"}},
		{name: "creation month day already passed not due", now: time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			due := model.DueDates(plan, tt.now, tt.ran)
			if len(due) != len(tt.want) {
				t.Fatalf("DueDates len = %d (%v), want %d (%v)", len(due), due, len(tt.want), tt.want)
			}
			for i, d := range due {
				if got := d.Format("2006-01-02"); got != tt.want[i] {
					t.Fatalf("DueDates[%d] = %s, want %s", i, got, tt.want[i])
				}
			}
		})
	}
}

func TestDueDatesMonthEndClamp(t *testing.T) {
	// day=31，2 月没有 31 日，执行日落到 2 月最后一天（2026 年 2 月 28 日）。
	plan := &model.RecurringPlan{DayOfMonth: 31, CreatedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.Local)}
	due := model.DueDates(plan, time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local), nil)
	if len(due) != 2 {
		t.Fatalf("DueDates len = %d (%v), want 2", len(due), due)
	}
	if got := due[0].Format("2006-01-02"); got != "2026-01-31" {
		t.Fatalf("DueDates[0] = %s, want 2026-01-31", got)
	}
	if got := due[1].Format("2006-01-02"); got != "2026-02-28" {
		t.Fatalf("DueDates[1] = %s, want 2026-02-28（月末无 31 日落到当月最后一天）", got)
	}
}

func TestRecurringPlanCreateAndNameUnique(t *testing.T) {
	_, svc, _, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	day := time.Now().Day()

	if _, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "房租", day)); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	tests := []struct {
		name    string
		plan    string
		wantErr bool
		errCode int
	}{
		{name: "duplicate name in same group", plan: "房租", wantErr: true, errCode: constants.CodePlanNameExists},
		{name: "another name ok", plan: "水电费", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, tt.plan, day))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if ae := util.AsAppError(err); ae.Code != tt.errCode {
					t.Fatalf("err code = %d, want %d", ae.Code, tt.errCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
		})
	}
}

func TestRecurringPlanGenerateIdempotent(t *testing.T) {
	db, svc, expenseSvc, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	now := time.Now()
	// 执行日定为今天，创建即到期生成一笔。
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "网费", now.Day()))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	period := now.Format("2006-01")
	if plan.LastRunPeriod != period {
		t.Fatalf("last_run_period = %q, want %q", plan.LastRunPeriod, period)
	}
	if n := countGroupExpenses(t, db, groupID); n != 1 {
		t.Fatalf("expense count = %d, want 1", n)
	}
	// 生成的消费沿用现有分摊逻辑：三人均摊 300，合计严格等于总额。
	expense, err := expenseSvc.Get(aliceID, *plan.LastExpenseID)
	if err != nil {
		t.Fatalf("get generated expense: %v", err)
	}
	sum := 0.0
	for _, sh := range expense.Shares {
		sum += sh.ShareAmount
	}
	if util.Round2(sum) != 300 {
		t.Fatalf("share sum = %.2f, want 300", sum)
	}
	if expense.PaidAt.Day() != now.Day() {
		t.Fatalf("paid_at day = %d, want %d", expense.PaidAt.Day(), now.Day())
	}

	// 重复查看列表/详情与连续手动触发，都不得重复入账。
	if _, _, err := svc.List(aliceID, groupID, &dto.RecurringPlanQuery{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, err := svc.Get(aliceID, plan.ID); err != nil {
		t.Fatalf("get: %v", err)
	}
	for i := 0; i < 3; i++ {
		periods, err := svc.Run(aliceID, plan.ID)
		if err != nil {
			t.Fatalf("run #%d: %v", i, err)
		}
		if len(periods) != 0 {
			t.Fatalf("run #%d generated %v, want empty（幂等）", i, periods)
		}
	}
	if n := countGroupExpenses(t, db, groupID); n != 1 {
		t.Fatalf("expense count after repeated triggers = %d, want 1（不得重复入账）", n)
	}
}

func TestRecurringPlanMonthEndServiceLevel(t *testing.T) {
	db, svc, _, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	created := time.Now()
	// day=31，注入时钟走到下个月 1 日：当月执行日应落到 min(31, 当月天数)。
	svc.now = func() time.Time {
		y, m, _ := created.Date()
		return time.Date(y, m+1, 1, 0, 0, 0, 0, time.Local)
	}
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "房贷", 31))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if n := countGroupExpenses(t, db, groupID); n != 1 {
		t.Fatalf("expense count = %d, want 1", n)
	}
	expense, err := svc.expenseSvc.Get(aliceID, *plan.LastExpenseID)
	if err != nil {
		t.Fatalf("get expense: %v", err)
	}
	want := model.EffectiveDate(created.Year(), created.Month(), 31)
	if expense.PaidAt.Day() != want.Day() || expense.PaidAt.Month() != want.Month() {
		t.Fatalf("paid_at = %v, want effective day %v（月末无对应日期落到当月最后一天）", expense.PaidAt, want)
	}
}

func TestRecurringPlanPausedNotGenerate(t *testing.T) {
	db, svc, _, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	now := time.Now()
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "物业费", now.Day()))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	before := countGroupExpenses(t, db, groupID)
	if before != 1 {
		t.Fatalf("expense count = %d, want 1", before)
	}
	if err := svc.Pause(aliceID, plan.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	// 时钟前进 40 天（跨月，必有新周期到期），停用计划不生成。
	svc.now = func() time.Time { return now.AddDate(0, 0, 40) }
	if _, _, err := svc.List(aliceID, groupID, &dto.RecurringPlanQuery{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if n := countGroupExpenses(t, db, groupID); n != before {
		t.Fatalf("paused plan generated expenses: count = %d, want %d（停用计划不生成）", n, before)
	}
	// 恢复后补跑到期周期。
	if err := svc.Resume(aliceID, plan.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if n := countGroupExpenses(t, db, groupID); n <= before {
		t.Fatalf("resumed plan did not catch up: count = %d, want > %d", n, before)
	}
	// 再次触发仍幂等。
	after := countGroupExpenses(t, db, groupID)
	if _, err := svc.Run(aliceID, plan.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := countGroupExpenses(t, db, groupID); n != after {
		t.Fatalf("count after re-run = %d, want %d", n, after)
	}
}

func TestRecurringPlanArchivedGroup(t *testing.T) {
	db, svc, _, groupSvc, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	now := time.Now()
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "保洁费", now.Day()))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := groupSvc.Archive(aliceID, groupID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	before := countGroupExpenses(t, db, groupID)

	tests := []struct {
		name    string
		fn      func() error
		errCode int
	}{
		{name: "create in archived group", fn: func() error {
			_, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "新计划", now.Day()))
			return err
		}, errCode: constants.CodeConflict},
		{name: "update in archived group", fn: func() error {
			return svc.Update(aliceID, plan.ID, &dto.UpdateRecurringPlanReq{
				Name: "保洁费2", Amount: 100, Category: "other", PayerID: aliceID,
				SplitType: "equal", DayOfMonth: 1, Shares: []dto.ShareInput{{UserID: aliceID}},
			})
		}, errCode: constants.CodeConflict},
		{name: "pause in archived group", fn: func() error { return svc.Pause(aliceID, plan.ID) }, errCode: constants.CodeConflict},
		{name: "remove in archived group", fn: func() error { return svc.Remove(aliceID, plan.ID) }, errCode: constants.CodeConflict},
		{name: "run in archived group", fn: func() error { _, err := svc.Run(aliceID, plan.ID); return err }, errCode: constants.CodeConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if ae := util.AsAppError(err); ae.Code != tt.errCode {
				t.Fatalf("err code = %d, want %d", ae.Code, tt.errCode)
			}
		})
	}
	// 归档群组不再生成（时钟前进也不生成）。
	svc.now = func() time.Time { return now.AddDate(0, 0, 40) }
	if _, _, err := svc.List(aliceID, groupID, &dto.RecurringPlanQuery{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if n := countGroupExpenses(t, db, groupID); n != before {
		t.Fatalf("archived group generated expenses: count = %d, want %d", n, before)
	}
}

func TestRecurringPlanRemoveKeepsExpenses(t *testing.T) {
	db, svc, expenseSvc, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	now := time.Now()
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "健身房", now.Day()))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	expenseID := *plan.LastExpenseID
	if err := svc.Remove(aliceID, plan.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// 已生成记录保持不变。
	expense, err := expenseSvc.Get(aliceID, expenseID)
	if err != nil {
		t.Fatalf("generated expense should be kept: %v", err)
	}
	if expense.Status != constants.ExpenseActive {
		t.Fatalf("expense status = %s, want active", expense.Status)
	}
	// 列表不再展示已移除计划。
	plans, total, err := svc.List(aliceID, groupID, &dto.RecurringPlanQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(plans) != 0 {
		t.Fatalf("list after remove = %d plans, want 0", total)
	}
	// 重复移除报状态冲突。
	if err := svc.Remove(aliceID, plan.ID); err == nil {
		t.Fatalf("expected conflict on re-remove, got nil")
	} else if ae := util.AsAppError(err); ae.Code != constants.CodePlanStatusConflict {
		t.Fatalf("err code = %d, want %d", ae.Code, constants.CodePlanStatusConflict)
	}
	// 移除后名称可复用。
	if _, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "健身房", now.Day())); err != nil {
		t.Fatalf("recreate with removed name: %v", err)
	}
	// 已移除计划不再生成。
	svc.now = func() time.Time { return now.AddDate(0, 0, 40) }
	if _, err := svc.Run(aliceID, plan.ID); err == nil {
		t.Fatalf("expected error running removed plan, got nil")
	}
	if n := countGroupExpenses(t, db, groupID); n != 2 {
		t.Fatalf("expense count = %d, want 2（旧计划 1 笔 + 新计划 1 笔）", n)
	}
}

func TestRecurringPlanUpdateAndNonMember(t *testing.T) {
	_, svc, _, _, groupID, aliceID, bobID, carolID := newPlanServiceFixture(t)
	now := time.Now()
	plan, err := svc.Create(aliceID, equalPlanReq(groupID, aliceID, aliceID, bobID, carolID, "宽带", now.Day()))
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	// 更新为按比例分摊。
	err = svc.Update(aliceID, plan.ID, &dto.UpdateRecurringPlanReq{
		Name: "宽带费", Amount: 120, Category: "other", PayerID: bobID,
		SplitType: "ratio", DayOfMonth: 5,
		Shares: []dto.ShareInput{{UserID: aliceID, Ratio: 1}, {UserID: bobID, Ratio: 2}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := svc.Get(aliceID, plan.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "宽带费" || got.DayOfMonth != 5 || got.PayerID != bobID || got.SplitType != constants.SplitRatio {
		t.Fatalf("plan after update = %+v", got)
	}
	if len(got.Shares) != 2 {
		t.Fatalf("shares len = %d, want 2", len(got.Shares))
	}
	// 非群组成员不可操作。
	if _, _, err := svc.List(999, groupID, &dto.RecurringPlanQuery{}); err == nil {
		t.Fatalf("expected not-member error, got nil")
	} else if ae := util.AsAppError(err); ae.Code != constants.CodeNotGroupMember {
		t.Fatalf("err code = %d, want %d", ae.Code, constants.CodeNotGroupMember)
	}
}
