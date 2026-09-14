package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
	"gorm.io/gorm"
)

// RecurringPlanService 周期账单计划业务逻辑：到期幂等生成消费记录。
type RecurringPlanService struct {
	db         *gorm.DB
	planRepo   *repository.RecurringPlanRepository
	groupRepo  *repository.GroupRepository
	memberRepo *repository.GroupMemberRepository
	expenseSvc *ExpenseService
	auditSvc   *AuditService
	logger     *slog.Logger
	now        func() time.Time // 可注入时钟，便于测试
}

// NewRecurringPlanService 构造周期账单计划服务。
func NewRecurringPlanService(db *gorm.DB, planRepo *repository.RecurringPlanRepository, groupRepo *repository.GroupRepository, memberRepo *repository.GroupMemberRepository, expenseSvc *ExpenseService, auditSvc *AuditService, logger *slog.Logger) *RecurringPlanService {
	return &RecurringPlanService{db: db, planRepo: planRepo, groupRepo: groupRepo, memberRepo: memberRepo, expenseSvc: expenseSvc, auditSvc: auditSvc, logger: logger, now: time.Now}
}

// Create 新建周期账单计划（事务 + 群组行锁；同群组名称唯一）。
func (s *RecurringPlanService) Create(userID uint, req *dto.CreateRecurringPlanReq) (*model.RecurringPlan, error) {
	var created *model.RecurringPlan
	err := s.db.Transaction(func(tx *gorm.DB) error {
		group, err := s.groupRepo.LockByIDTx(tx, req.GroupID)
		if err != nil {
			return util.Wrap(constants.CodeNotFound, "群组 group 不存在", err)
		}
		if !group.IsActive() {
			return util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法新建周期账单计划 recurring_plan", nil)
		}
		if err := s.ensureMember(req.GroupID, userID); err != nil {
			return err
		}
		if err := s.ensureMember(req.GroupID, req.PayerID); err != nil {
			return util.NewAppError(constants.CodeBadRequest, fmt.Sprintf("付款人 payer_id=%d 不是群组成员 member", req.PayerID), err)
		}
		if err := s.expenseSvc.ValidateShares(req.GroupID, req.Amount, req.SplitType, req.Shares); err != nil {
			return err
		}
		exists, err := s.planRepo.ExistsName(tx, req.GroupID, req.Name, 0)
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		if exists {
			return util.NewAppError(constants.CodePlanNameExists, fmt.Sprintf("周期账单计划 recurring_plan 名称 name=%s 在群组 group_id=%d 内已存在", req.Name, req.GroupID), nil)
		}
		plan := &model.RecurringPlan{
			GroupID:    req.GroupID,
			Name:       req.Name,
			Amount:     util.Round2(req.Amount),
			Category:   constants.ExpenseCategory(req.Category),
			PayerID:    req.PayerID,
			SplitType:  constants.SplitType(req.SplitType),
			DayOfMonth: req.DayOfMonth,
			Status:     constants.PlanActive,
			CreatedBy:  userID,
		}
		if err := s.planRepo.Create(tx, plan); err != nil {
			return err
		}
		if err := s.planRepo.CreateShares(tx, buildPlanShares(plan.ID, req.Shares)); err != nil {
			return err
		}
		created = plan
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(fmt.Sprintf(constants.LogPlanCreated, created.ID, created.GroupID, created.Name, created.Amount, created.DayOfMonth, userID))
	s.auditSvc.Record(userID, string(constants.ActionPlanCreate), "recurring_plan", fmt.Sprint(created.ID), "创建周期账单计划 "+created.Name, "")
	// 创建当日即达执行日时立即生成（幂等，重复触发不会重复入账）
	if _, err := s.generateForPlan(userID, created.ID); err != nil {
		s.logger.Warn(fmt.Sprintf(constants.LogPlanGenerateSkipped, created.ID, err.Error()))
	}
	return s.planRepo.FindByID(created.ID)
}

// Update 更新周期账单计划（事务 + 群组行锁；归档群组与已移除计划不可修改）。
func (s *RecurringPlanService) Update(operatorID, planID uint, req *dto.UpdateRecurringPlanReq) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		plan, group, err := s.lockPlanAndGroup(tx, planID)
		if err != nil {
			return err
		}
		if !group.IsActive() {
			return util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法修改周期账单计划 recurring_plan", nil)
		}
		if err := s.ensureMember(plan.GroupID, operatorID); err != nil {
			return err
		}
		if plan.IsRemoved() {
			return util.NewAppError(constants.CodePlanStatusConflict, constants.MsgErrPlanRemoved, nil)
		}
		if err := s.ensureMember(plan.GroupID, req.PayerID); err != nil {
			return util.NewAppError(constants.CodeBadRequest, fmt.Sprintf("付款人 payer_id=%d 不是群组成员 member", req.PayerID), err)
		}
		if err := s.expenseSvc.ValidateShares(plan.GroupID, req.Amount, req.SplitType, req.Shares); err != nil {
			return err
		}
		exists, err := s.planRepo.ExistsName(tx, plan.GroupID, req.Name, planID)
		if err != nil {
			return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
		}
		if exists {
			return util.NewAppError(constants.CodePlanNameExists, fmt.Sprintf("周期账单计划 recurring_plan 名称 name=%s 在群组 group_id=%d 内已存在", req.Name, plan.GroupID), nil)
		}
		plan.Name = req.Name
		plan.Amount = util.Round2(req.Amount)
		plan.Category = constants.ExpenseCategory(req.Category)
		plan.PayerID = req.PayerID
		plan.SplitType = constants.SplitType(req.SplitType)
		plan.DayOfMonth = req.DayOfMonth
		if err := s.planRepo.Update(tx, plan); err != nil {
			return err
		}
		if err := s.planRepo.DeleteShares(tx, planID); err != nil {
			return err
		}
		return s.planRepo.CreateShares(tx, buildPlanShares(planID, req.Shares))
	})
	if err != nil {
		return err
	}
	s.logger.Info(fmt.Sprintf(constants.LogPlanUpdated, planID, req.Name, req.Amount, req.DayOfMonth, operatorID))
	s.auditSvc.Record(operatorID, string(constants.ActionPlanUpdate), "recurring_plan", fmt.Sprint(planID), "更新周期账单计划 "+req.Name, "")
	return nil
}

// Pause 暂停周期账单计划（暂停期间不生成消费）。
func (s *RecurringPlanService) Pause(operatorID, planID uint) error {
	return s.changeStatus(operatorID, planID, constants.PlanPaused)
}

// Resume 恢复周期账单计划（恢复后立即补跑到期周期，幂等）。
func (s *RecurringPlanService) Resume(operatorID, planID uint) error {
	if err := s.changeStatus(operatorID, planID, constants.PlanActive); err != nil {
		return err
	}
	if _, err := s.generateForPlan(operatorID, planID); err != nil {
		s.logger.Warn(fmt.Sprintf(constants.LogPlanGenerateSkipped, planID, err.Error()))
	}
	return nil
}

// Remove 移除周期账单计划（软删除；已生成的消费记录保持不变）。
func (s *RecurringPlanService) Remove(operatorID, planID uint) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		plan, group, err := s.lockPlanAndGroup(tx, planID)
		if err != nil {
			return err
		}
		if !group.IsActive() {
			return util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法修改周期账单计划 recurring_plan", nil)
		}
		if err := s.ensureMember(plan.GroupID, operatorID); err != nil {
			return err
		}
		if plan.IsRemoved() {
			return util.NewAppError(constants.CodePlanStatusConflict, constants.MsgErrPlanRemoved, nil)
		}
		return s.planRepo.UpdateFields(tx, planID, map[string]interface{}{"status": constants.PlanRemoved})
	})
	if err != nil {
		return err
	}
	s.logger.Info(fmt.Sprintf(constants.LogPlanRemoved, planID, operatorID))
	s.auditSvc.Record(operatorID, string(constants.ActionPlanRemove), "recurring_plan", fmt.Sprint(planID), "移除周期账单计划", "")
	return nil
}

// Get 查询计划详情（触发该计划的到期生成后返回最新状态）。
func (s *RecurringPlanService) Get(userID, planID uint) (*model.RecurringPlan, error) {
	plan, err := s.planRepo.FindByID(planID)
	if errors.Is(err, repository.ErrRecurringPlanNotFound) {
		return nil, util.NewAppError(constants.CodeNotFound, "周期账单计划 recurring_plan 不存在", err)
	}
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if err := s.ensureMember(plan.GroupID, userID); err != nil {
		return nil, err
	}
	if _, err := s.generateForPlan(userID, planID); err != nil {
		s.logger.Warn(fmt.Sprintf(constants.LogPlanGenerateSkipped, planID, err.Error()))
	}
	return s.planRepo.FindByID(planID)
}

// List 分页查询群组计划（先对群组全部启用中计划做到期生成，再返回列表）。
func (s *RecurringPlanService) List(userID, groupID uint, query *dto.RecurringPlanQuery) ([]model.RecurringPlan, int64, error) {
	if err := s.ensureMember(groupID, userID); err != nil {
		return nil, 0, err
	}
	s.generateDueForGroup(userID, groupID)
	page, pageSize := query.Page, query.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = util.DefaultPageSize
	}
	return s.planRepo.ListByGroup(groupID, query.Status, page, pageSize)
}

// Run 手动触发单个计划的到期生成（连续触发幂等，不会重复入账）。
func (s *RecurringPlanService) Run(operatorID, planID uint) ([]string, error) {
	plan, err := s.planRepo.FindByID(planID)
	if errors.Is(err, repository.ErrRecurringPlanNotFound) {
		return nil, util.NewAppError(constants.CodeNotFound, "周期账单计划 recurring_plan 不存在", err)
	}
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if err := s.ensureMember(plan.GroupID, operatorID); err != nil {
		return nil, err
	}
	group, err := s.groupRepo.FindByID(plan.GroupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeNotFound, "群组 group 不存在", err)
	}
	if !group.IsActive() {
		return nil, util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法执行周期账单计划 recurring_plan", nil)
	}
	if !plan.IsActive() {
		return nil, util.NewAppError(constants.CodePlanStatusConflict, "周期账单计划 recurring_plan 未启用，无法执行", nil)
	}
	periods, err := s.generateForPlan(operatorID, planID)
	if err != nil {
		return nil, err
	}
	s.auditSvc.Record(operatorID, string(constants.ActionPlanRun), "recurring_plan", fmt.Sprint(planID), fmt.Sprintf("手动触发周期账单计划，生成 %d 个周期", len(periods)), "")
	return periods, nil
}

// changeStatus 暂停/恢复状态流转（事务 + 行锁）。
func (s *RecurringPlanService) changeStatus(operatorID, planID uint, target constants.RecurringPlanStatus) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		plan, group, err := s.lockPlanAndGroup(tx, planID)
		if err != nil {
			return err
		}
		if !group.IsActive() {
			return util.NewAppError(constants.CodeConflict, "群组 group 已归档，无法修改周期账单计划 recurring_plan", nil)
		}
		if err := s.ensureMember(plan.GroupID, operatorID); err != nil {
			return err
		}
		if plan.IsRemoved() {
			return util.NewAppError(constants.CodePlanStatusConflict, constants.MsgErrPlanRemoved, nil)
		}
		if plan.Status == target {
			return util.NewAppError(constants.CodePlanStatusConflict, fmt.Sprintf("周期账单计划 recurring_plan 已处于状态 status=%s", target), nil)
		}
		return s.planRepo.UpdateFields(tx, planID, map[string]interface{}{"status": target})
	})
	if err != nil {
		return err
	}
	if target == constants.PlanPaused {
		s.logger.Info(fmt.Sprintf(constants.LogPlanPaused, planID, operatorID))
		s.auditSvc.Record(operatorID, string(constants.ActionPlanPause), "recurring_plan", fmt.Sprint(planID), "暂停周期账单计划", "")
	} else {
		s.logger.Info(fmt.Sprintf(constants.LogPlanResumed, planID, operatorID))
		s.auditSvc.Record(operatorID, string(constants.ActionPlanResume), "recurring_plan", fmt.Sprint(planID), "恢复周期账单计划", "")
	}
	return nil
}

// generateDueForGroup 对群组全部启用中计划做到期生成（尽力而为，单计划失败不影响其他计划）。
func (s *RecurringPlanService) generateDueForGroup(operatorID, groupID uint) {
	group, err := s.groupRepo.FindByID(groupID)
	if err != nil || !group.IsActive() {
		return // 归档群组不生成
	}
	plans, err := s.planRepo.ListActiveByGroup(groupID)
	if err != nil {
		return
	}
	for i := range plans {
		if _, err := s.generateForPlan(operatorID, plans[i].ID); err != nil {
			s.logger.Warn(fmt.Sprintf(constants.LogPlanGenerateSkipped, plans[i].ID, err.Error()))
		}
	}
}

// generateForPlan 生成单个计划全部到期周期的消费（每个周期事务 + 计划行锁 + 周期唯一索引，三重幂等）。
// 返回本次生成的周期列表；重复查看或连续触发不会重复入账。
func (s *RecurringPlanService) generateForPlan(operatorID, planID uint) ([]string, error) {
	plan, err := s.planRepo.FindByID(planID)
	if errors.Is(err, repository.ErrRecurringPlanNotFound) {
		return nil, util.NewAppError(constants.CodeNotFound, "周期账单计划 recurring_plan 不存在", err)
	}
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if !plan.IsActive() {
		return nil, nil // 停用计划不生成
	}
	group, err := s.groupRepo.FindByID(plan.GroupID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if !group.IsActive() {
		return nil, nil // 归档群组不生成
	}
	ran, err := s.planRepo.ListRanPeriods(planID)
	if err != nil {
		return nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	dueDates := model.DueDates(plan, s.now(), ran)
	generated := make([]string, 0, len(dueDates))
	for _, due := range dueDates {
		period := model.PeriodOf(due)
		var expense *model.Expense
		err := s.db.Transaction(func(tx *gorm.DB) error {
			locked, err := s.planRepo.LockByID(tx, planID)
			if err != nil {
				return err
			}
			if !locked.IsActive() {
				return nil // 并发下被暂停/移除，放弃本次生成
			}
			exists, err := s.planRepo.RunExists(tx, planID, period)
			if err != nil {
				return err
			}
			if exists {
				return nil // 锁内复查：该周期已生成，幂等跳过
			}
			expense, err = s.expenseSvc.CreateWithTx(tx, locked.CreatedBy, buildExpenseReq(locked, due, period))
			if err != nil {
				return err
			}
			run := &model.RecurringPlanRun{PlanID: planID, Period: period, ExpenseID: expense.ID, GeneratedAt: s.now()}
			if err := s.planRepo.CreateRun(tx, run); err != nil {
				if isDuplicateKeyErr(err) {
					return nil // 唯一索引兜底：并发下已被其他请求生成
				}
				return err
			}
			return s.planRepo.UpdateFields(tx, planID, map[string]interface{}{
				"last_run_period":   period,
				"last_expense_id":   expense.ID,
				"last_generated_at": s.now(),
			})
		})
		if err != nil {
			return generated, err
		}
		if expense != nil {
			generated = append(generated, period)
			s.logger.Info(fmt.Sprintf(constants.LogPlanExpenseGenerated, planID, period, expense.ID, expense.Amount))
			s.auditSvc.Record(operatorID, string(constants.ActionPlanGenerate), "recurring_plan", fmt.Sprint(planID), fmt.Sprintf("周期账单计划生成 %s 期消费 expense_id=%d", period, expense.ID), "")
		}
	}
	return generated, nil
}

// lockPlanAndGroup 事务内依次锁定计划与群组（固定顺序避免死锁）。
func (s *RecurringPlanService) lockPlanAndGroup(tx *gorm.DB, planID uint) (*model.RecurringPlan, *model.Group, error) {
	plan, err := s.planRepo.LockByID(tx, planID)
	if errors.Is(err, repository.ErrRecurringPlanNotFound) {
		return nil, nil, util.NewAppError(constants.CodeNotFound, "周期账单计划 recurring_plan 不存在", err)
	}
	if err != nil {
		return nil, nil, util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	group, err := s.groupRepo.LockByIDTx(tx, plan.GroupID)
	if err != nil {
		return nil, nil, util.Wrap(constants.CodeNotFound, "群组 group 不存在", err)
	}
	return plan, group, nil
}

// ensureMember 校验用户是群组成员。
func (s *RecurringPlanService) ensureMember(groupID, userID uint) error {
	ok, err := s.memberRepo.Exists(groupID, userID)
	if err != nil {
		return util.Wrap(constants.CodeInternalError, constants.MsgErrInternal, err)
	}
	if !ok {
		return util.NewAppError(constants.CodeNotGroupMember, fmt.Sprintf("用户 user_id=%d 不是群组 group_id=%d 的成员 member", userID, groupID), nil)
	}
	return nil
}

// buildPlanShares 组装计划参与人明细。
func buildPlanShares(planID uint, inputs []dto.ShareInput) []model.RecurringPlanShare {
	shares := make([]model.RecurringPlanShare, 0, len(inputs))
	for _, in := range inputs {
		shares = append(shares, model.RecurringPlanShare{PlanID: planID, UserID: in.UserID, Ratio: in.Ratio, Amount: in.Amount})
	}
	return shares
}

// buildExpenseReq 由计划组装消费创建请求（复用消费分摊与结算逻辑）。
func buildExpenseReq(plan *model.RecurringPlan, due time.Time, period string) *dto.CreateExpenseReq {
	shares := make([]dto.ShareInput, 0, len(plan.Shares))
	for _, sh := range plan.Shares {
		shares = append(shares, dto.ShareInput{UserID: sh.UserID, Ratio: sh.Ratio, Amount: sh.Amount})
	}
	paidAt := time.Date(due.Year(), due.Month(), due.Day(), 9, 0, 0, 0, time.Local)
	return &dto.CreateExpenseReq{
		GroupID:   plan.GroupID,
		Title:     fmt.Sprintf("%s（%s）", plan.Name, period),
		Amount:    plan.Amount,
		Category:  string(plan.Category),
		PayerID:   plan.PayerID,
		SplitType: string(plan.SplitType),
		PaidAt:    paidAt.Format("2006-01-02 15:04:05"),
		Shares:    shares,
	}
}

// isDuplicateKeyErr 判断唯一索引冲突（PostgreSQL 23505 / SQLite UNIQUE constraint）。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "UNIQUE constraint")
}
