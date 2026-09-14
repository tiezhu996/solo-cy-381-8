package handler

import (
	"time"

	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/service"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/gin-gonic/gin"
)

// RecurringPlanHandler 周期账单计划 HTTP 处理器。
type RecurringPlanHandler struct {
	planSvc *service.RecurringPlanService
}

// NewRecurringPlanHandler 构造周期账单计划处理器。
func NewRecurringPlanHandler(planSvc *service.RecurringPlanService) *RecurringPlanHandler {
	return &RecurringPlanHandler{planSvc: planSvc}
}

// Create 新建周期账单计划。
func (h *RecurringPlanHandler) Create(c *gin.Context) {
	var req dto.CreateRecurringPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, util.Wrap(util.ValidationCode(err), util.ValidationMessage(err), err))
		return
	}
	if req.GroupID == 0 {
		req.GroupID = parseIDParam(c)
	}
	plan, err := h.planSvc.Create(util.GetUserID(c), &req)
	if err != nil {
		util.Fail(c, err)
		return
	}
	util.Created(c, toRecurringPlanResp(plan))
}

// Update 更新周期账单计划。
func (h *RecurringPlanHandler) Update(c *gin.Context) {
	var req dto.UpdateRecurringPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, util.Wrap(util.ValidationCode(err), util.ValidationMessage(err), err))
		return
	}
	if err := h.planSvc.Update(util.GetUserID(c), parseIDParam(c), &req); err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "周期账单计划更新成功"})
}

// Pause 暂停周期账单计划。
func (h *RecurringPlanHandler) Pause(c *gin.Context) {
	if err := h.planSvc.Pause(util.GetUserID(c), parseIDParam(c)); err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "周期账单计划已暂停"})
}

// Resume 恢复周期账单计划。
func (h *RecurringPlanHandler) Resume(c *gin.Context) {
	if err := h.planSvc.Resume(util.GetUserID(c), parseIDParam(c)); err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "周期账单计划已恢复"})
}

// Remove 移除周期账单计划（已生成消费记录保持不变）。
func (h *RecurringPlanHandler) Remove(c *gin.Context) {
	if err := h.planSvc.Remove(util.GetUserID(c), parseIDParam(c)); err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, gin.H{"message": "周期账单计划已移除"})
}

// Run 手动触发到期生成（连续触发幂等）。
func (h *RecurringPlanHandler) Run(c *gin.Context) {
	periods, err := h.planSvc.Run(util.GetUserID(c), parseIDParam(c))
	if err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, dto.RecurringPlanRunResp{Generated: len(periods), Periods: periods})
}

// Get 查询周期账单计划详情。
func (h *RecurringPlanHandler) Get(c *gin.Context) {
	plan, err := h.planSvc.Get(util.GetUserID(c), parseIDParam(c))
	if err != nil {
		util.Fail(c, err)
		return
	}
	util.OK(c, toRecurringPlanResp(plan))
}

// List 分页查询群组周期账单计划。
func (h *RecurringPlanHandler) List(c *gin.Context) {
	groupID := parseIDParam(c)
	var query dto.RecurringPlanQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		util.Fail(c, util.Wrap(util.ValidationCode(err), util.ValidationMessage(err), err))
		return
	}
	plans, total, err := h.planSvc.List(util.GetUserID(c), groupID, &query)
	if err != nil {
		util.Fail(c, err)
		return
	}
	list := make([]*dto.RecurringPlanResp, 0, len(plans))
	for i := range plans {
		list = append(list, toRecurringPlanResp(&plans[i]))
	}
	page, pageSize := util.ParsePagination(c)
	util.OK(c, util.PageData{List: list, Total: total, Page: page, PageSize: pageSize})
}

// toRecurringPlanResp 周期账单计划模型转响应（含下次执行日与上次生成结果）。
func toRecurringPlanResp(p *model.RecurringPlan) *dto.RecurringPlanResp {
	resp := &dto.RecurringPlanResp{
		ID:          p.ID,
		GroupID:     p.GroupID,
		Name:        p.Name,
		Amount:      util.Round2(p.Amount),
		Category:    string(p.Category),
		PayerID:     p.PayerID,
		PayerName:   p.PayerName(),
		SplitType:   string(p.SplitType),
		DayOfMonth:  p.DayOfMonth,
		Status:      string(p.Status),
		StatusText:  util.RecurringPlanStatusText(string(p.Status)),
		NextRunDate: util.FormatDate(model.NextRunDate(p.DayOfMonth, time.Now())),
		CreatedBy:   p.CreatedBy,
		CreatedAt:   util.FormatDateTime(p.CreatedAt),
		Shares:      make([]dto.RecurringPlanShareResp, 0, len(p.Shares)),
	}
	for _, s := range p.Shares {
		username, nickname := "", ""
		if s.User != nil {
			username = s.User.Username
			nickname = s.User.Nickname
		}
		resp.Shares = append(resp.Shares, dto.RecurringPlanShareResp{
			UserID:   s.UserID,
			Username: username,
			Nickname: nickname,
			Ratio:    util.Round2(s.Ratio),
			Amount:   util.Round2(s.Amount),
		})
	}
	if p.LastRunPeriod != "" && p.LastExpenseID != nil && p.LastGeneratedAt != nil {
		last := &dto.RecurringPlanLastRunResp{
			Period:      p.LastRunPeriod,
			ExpenseID:   *p.LastExpenseID,
			Amount:      util.Round2(p.Amount),
			GeneratedAt: util.FormatDateTime(*p.LastGeneratedAt),
		}
		if p.LastExpense != nil {
			last.ExpenseTitle = p.LastExpense.Title
			last.Amount = util.Round2(p.LastExpense.Amount)
		}
		resp.LastRun = last
	}
	return resp
}
