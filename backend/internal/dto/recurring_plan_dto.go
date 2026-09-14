package dto

// CreateRecurringPlanReq 创建周期账单计划请求（group_id 可省略，由路径参数补充）。
type CreateRecurringPlanReq struct {
	GroupID    uint         `json:"group_id" binding:"omitempty,gt=0"`
	Name       string       `json:"name" binding:"required,min=1,max=128"`
	Amount     float64      `json:"amount" binding:"required,gt=0"`
	Category   string       `json:"category" binding:"required,oneof=dining transport lodging entertain other"`
	PayerID    uint         `json:"payer_id" binding:"required,gt=0"`
	SplitType  string       `json:"split_type" binding:"required,oneof=equal ratio amount"`
	DayOfMonth int          `json:"day_of_month" binding:"required,gte=1,lte=31"`
	Shares     []ShareInput `json:"shares" binding:"required,min=1"`
}

// UpdateRecurringPlanReq 更新周期账单计划请求。
type UpdateRecurringPlanReq struct {
	Name       string       `json:"name" binding:"required,min=1,max=128"`
	Amount     float64      `json:"amount" binding:"required,gt=0"`
	Category   string       `json:"category" binding:"required,oneof=dining transport lodging entertain other"`
	PayerID    uint         `json:"payer_id" binding:"required,gt=0"`
	SplitType  string       `json:"split_type" binding:"required,oneof=equal ratio amount"`
	DayOfMonth int          `json:"day_of_month" binding:"required,gte=1,lte=31"`
	Shares     []ShareInput `json:"shares" binding:"required,min=1"`
}

// RecurringPlanQuery 周期账单计划筛选查询。
type RecurringPlanQuery struct {
	Page     int    `form:"page" binding:"omitempty,gte=1"`
	PageSize int    `form:"page_size" binding:"omitempty,gte=1,lte=100"`
	Status   string `form:"status" binding:"omitempty,oneof=active paused"`
}

// RecurringPlanShareResp 计划参与人响应。
type RecurringPlanShareResp struct {
	UserID   uint    `json:"user_id"`
	Username string  `json:"username"`
	Nickname string  `json:"nickname"`
	Ratio    float64 `json:"ratio"`
	Amount   float64 `json:"amount"`
}

// RecurringPlanLastRunResp 上次生成结果响应。
type RecurringPlanLastRunResp struct {
	Period       string  `json:"period"`
	ExpenseID    uint    `json:"expense_id"`
	ExpenseTitle string  `json:"expense_title"`
	Amount       float64 `json:"amount"`
	GeneratedAt  string  `json:"generated_at"`
}

// RecurringPlanResp 周期账单计划响应。
type RecurringPlanResp struct {
	ID          uint                      `json:"id"`
	GroupID     uint                      `json:"group_id"`
	Name        string                    `json:"name"`
	Amount      float64                   `json:"amount"`
	Category    string                    `json:"category"`
	PayerID     uint                      `json:"payer_id"`
	PayerName   string                    `json:"payer_name"`
	SplitType   string                    `json:"split_type"`
	DayOfMonth  int                       `json:"day_of_month"`
	Status      string                    `json:"status"`
	StatusText  string                    `json:"status_text"`
	NextRunDate string                    `json:"next_run_date"`
	LastRun     *RecurringPlanLastRunResp `json:"last_run"`
	CreatedBy   uint                      `json:"created_by"`
	CreatedAt   string                    `json:"created_at"`
	Shares      []RecurringPlanShareResp  `json:"shares"`
}

// RecurringPlanRunResp 手动触发生成的结果响应。
type RecurringPlanRunResp struct {
	Generated int      `json:"generated"`
	Periods   []string `json:"periods"`
}
