package repository

import (
	"errors"
	"fmt"

	"github.com/aasplit/aasplit/internal/model"
	"gorm.io/gorm"
)

// ErrRecurringPlanNotFound 周期账单计划不存在哨兵错误。
var ErrRecurringPlanNotFound = errors.New("recurring plan not found")

// RecurringPlanRepository 周期账单计划数据访问。
type RecurringPlanRepository struct {
	db *gorm.DB
}

// NewRecurringPlanRepository 构造周期账单计划仓储。
func NewRecurringPlanRepository(db *gorm.DB) *RecurringPlanRepository {
	return &RecurringPlanRepository{db: db}
}

// Create 创建周期账单计划（事务中调用）。
func (r *RecurringPlanRepository) Create(tx *gorm.DB, plan *model.RecurringPlan) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Create(plan).Error; err != nil {
		return fmt.Errorf("create recurring plan: %w", err)
	}
	return nil
}

// CreateShares 批量创建计划参与人明细（事务中调用）。
func (r *RecurringPlanRepository) CreateShares(tx *gorm.DB, shares []model.RecurringPlanShare) error {
	if len(shares) == 0 {
		return nil
	}
	if tx == nil {
		tx = r.db
	}
	if err := tx.Create(&shares).Error; err != nil {
		return fmt.Errorf("create recurring plan shares: %w", err)
	}
	return nil
}

// DeleteShares 删除某计划的全部参与人明细（事务中调用）。
func (r *RecurringPlanRepository) DeleteShares(tx *gorm.DB, planID uint) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Where("plan_id = ?", planID).Delete(&model.RecurringPlanShare{}).Error; err != nil {
		return fmt.Errorf("delete recurring plan shares: %w", err)
	}
	return nil
}

// FindByID 按 ID 查询（含付款人、参与人与上次生成的消费）。
func (r *RecurringPlanRepository) FindByID(id uint) (*model.RecurringPlan, error) {
	var p model.RecurringPlan
	if err := r.db.Preload("Payer").Preload("Shares.User").Preload("LastExpense").First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecurringPlanNotFound
		}
		return nil, fmt.Errorf("find recurring plan by id: %w", err)
	}
	return &p, nil
}

// LockByID 事务内行级锁查询计划（并发触发生成时串行化，SELECT ... FOR UPDATE）。
func (r *RecurringPlanRepository) LockByID(tx *gorm.DB, id uint) (*model.RecurringPlan, error) {
	if tx == nil {
		tx = r.db
	}
	var p model.RecurringPlan
	if err := tx.Clauses(lockClause).Preload("Shares").First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecurringPlanNotFound
		}
		return nil, fmt.Errorf("lock recurring plan by id: %w", err)
	}
	return &p, nil
}

// Update 更新计划主体字段（事务中调用）。
func (r *RecurringPlanRepository) Update(tx *gorm.DB, plan *model.RecurringPlan) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Save(plan).Error; err != nil {
		return fmt.Errorf("update recurring plan: %w", err)
	}
	return nil
}

// UpdateFields 按字段图更新计划（事务中调用）。
func (r *RecurringPlanRepository) UpdateFields(tx *gorm.DB, id uint, fields map[string]interface{}) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Model(&model.RecurringPlan{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		return fmt.Errorf("update recurring plan fields: %w", err)
	}
	return nil
}

// ExistsName 判断群组内是否已存在同名计划（已移除的不参与查重）。
func (r *RecurringPlanRepository) ExistsName(tx *gorm.DB, groupID uint, name string, excludeID uint) (bool, error) {
	if tx == nil {
		tx = r.db
	}
	var n int64
	query := tx.Model(&model.RecurringPlan{}).
		Where("group_id = ? AND name = ? AND status <> ?", groupID, name, "removed")
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&n).Error; err != nil {
		return false, fmt.Errorf("count recurring plan name: %w", err)
	}
	return n > 0, nil
}

// ListByGroup 分页查询群组计划（默认不含已移除，含付款人、参与人与上次生成的消费）。
func (r *RecurringPlanRepository) ListByGroup(groupID uint, status string, page, pageSize int) ([]model.RecurringPlan, int64, error) {
	var plans []model.RecurringPlan
	var total int64
	query := r.db.Model(&model.RecurringPlan{}).Where("group_id = ?", groupID)
	if status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status <> ?", "removed")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count recurring plans: %w", err)
	}
	if err := query.Preload("Payer").Preload("Shares.User").Preload("LastExpense").
		Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&plans).Error; err != nil {
		return nil, 0, fmt.Errorf("list recurring plans: %w", err)
	}
	return plans, total, nil
}

// ListActiveByGroup 查询群组全部启用中的计划（列表页惰性触发生成用）。
func (r *RecurringPlanRepository) ListActiveByGroup(groupID uint) ([]model.RecurringPlan, error) {
	var plans []model.RecurringPlan
	if err := r.db.Where("group_id = ? AND status = ?", groupID, "active").Find(&plans).Error; err != nil {
		return nil, fmt.Errorf("list active recurring plans: %w", err)
	}
	return plans, nil
}

// CreateRun 写入计划生成记录（plan_id+period 唯一索引兜底幂等，事务中调用）。
func (r *RecurringPlanRepository) CreateRun(tx *gorm.DB, run *model.RecurringPlanRun) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Create(run).Error; err != nil {
		return fmt.Errorf("create recurring plan run: %w", err)
	}
	return nil
}

// RunExists 在行锁内复查某周期是否已生成（幂等第二道防线，事务中调用）。
func (r *RecurringPlanRepository) RunExists(tx *gorm.DB, planID uint, period string) (bool, error) {
	if tx == nil {
		tx = r.db
	}
	var n int64
	if err := tx.Model(&model.RecurringPlanRun{}).Where("plan_id = ? AND period = ?", planID, period).Count(&n).Error; err != nil {
		return false, fmt.Errorf("count recurring plan run: %w", err)
	}
	return n > 0, nil
}

// ListRanPeriods 查询计划已生成的全部周期（用于到期计算去重）。
func (r *RecurringPlanRepository) ListRanPeriods(planID uint) (map[string]bool, error) {
	var periods []string
	if err := r.db.Model(&model.RecurringPlanRun{}).Where("plan_id = ?", planID).Pluck("period", &periods).Error; err != nil {
		return nil, fmt.Errorf("list recurring plan ran periods: %w", err)
	}
	ran := make(map[string]bool, len(periods))
	for _, p := range periods {
		ran[p] = true
	}
	return ran, nil
}
