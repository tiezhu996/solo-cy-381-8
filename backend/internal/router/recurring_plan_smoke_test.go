package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aasplit/aasplit/internal/config"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// smokeResp 统一响应。
type smokeResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// TestRecurringPlanHTTPSmoke 临时冒烟测试：注册→建群→建计划→列表→暂停/恢复/触发/移除。
func TestRecurringPlanHTTPSmoke(t *testing.T) {
	dsn := fmt.Sprintf("file:smoke%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Group{}, &model.GroupMember{}, &model.Expense{}, &model.ExpenseShare{},
		&model.Settlement{}, &model.AuditLog{}, &model.RecurringPlan{}, &model.RecurringPlanShare{}, &model.RecurringPlanRun{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{AppName: "smoke", JWTSecret: "smoke-secret-0123456789", JWTExpireH: 72, RateLimit: 10000, CORSOrigins: "*"}
	engine := Setup(cfg, db, nil, util.NewLogger())

	do := func(method, path, token string, body any) (int, smokeResp) {
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatalf("encode: %v", err)
			}
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		var resp smokeResp
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	// 注册并登录
	if code, resp := do("POST", "/api/v1/auth/register", "", map[string]any{"username": "smoke", "password": "secret123", "nickname": "Smoke"}); code >= 300 {
		t.Fatalf("register: %d %s", code, resp.Message)
	}
	code, resp := do("POST", "/api/v1/auth/login", "", map[string]any{"username": "smoke", "password": "secret123"})
	if code >= 300 {
		t.Fatalf("login: %d %s", code, resp.Message)
	}
	var loginData struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(resp.Data, &loginData)
	token := loginData.Token
	if token == "" {
		t.Fatalf("empty token")
	}

	// 建群
	code, resp = do("POST", "/api/v1/groups", token, map[string]any{"name": "冒烟群"})
	if code >= 300 {
		t.Fatalf("create group: %d %s", code, resp.Message)
	}
	var group struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(resp.Data, &group)

	// 建计划（执行日=今天，创建即到期生成一笔）
	day := time.Now().Day()
	planBody := map[string]any{
		"name": "房租", "amount": 3000, "category": "lodging", "payer_id": 1,
		"split_type": "equal", "day_of_month": day, "shares": []map[string]any{{"user_id": 1}},
	}
	code, resp = do("POST", fmt.Sprintf("/api/v1/groups/%d/plans", group.ID), token, planBody)
	if code >= 300 {
		t.Fatalf("create plan: %d %s", code, resp.Message)
	}
	var plan struct {
		ID          uint   `json:"id"`
		NextRunDate string `json:"next_run_date"`
		LastRun     *struct {
			Period string `json:"period"`
		} `json:"last_run"`
	}
	_ = json.Unmarshal(resp.Data, &plan)
	if plan.ID == 0 {
		t.Fatalf("plan id = 0")
	}
	if plan.LastRun == nil || plan.LastRun.Period != time.Now().Format("2006-01") {
		t.Fatalf("last_run = %+v, want current period", plan.LastRun)
	}
	if plan.NextRunDate == "" {
		t.Fatalf("next_run_date empty")
	}

	// 重名计划应 409
	if code, _ = do("POST", fmt.Sprintf("/api/v1/groups/%d/plans", group.ID), token, planBody); code != 409 {
		t.Fatalf("duplicate plan name: code = %d, want 409", code)
	}

	// 列表（应只有 1 条，且触发幂等不重复入账）
	code, resp = do("GET", fmt.Sprintf("/api/v1/groups/%d/plans", group.ID), token, nil)
	if code >= 300 {
		t.Fatalf("list plans: %d", code)
	}
	var listData struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(resp.Data, &listData)
	if listData.Total != 1 {
		t.Fatalf("plans total = %d, want 1", listData.Total)
	}

	// 连续手动触发幂等
	for i := 0; i < 2; i++ {
		code, resp = do("POST", fmt.Sprintf("/api/v1/plans/%d/run", plan.ID), token, nil)
		if code >= 300 {
			t.Fatalf("run plan: %d", code)
		}
		var runData struct {
			Generated int `json:"generated"`
		}
		_ = json.Unmarshal(resp.Data, &runData)
		if runData.Generated != 0 {
			t.Fatalf("run #%d generated = %d, want 0（幂等）", i, runData.Generated)
		}
	}

	// 消费列表应只有 1 笔自动生成的消费
	code, resp = do("GET", fmt.Sprintf("/api/v1/groups/%d/expenses", group.ID), token, nil)
	if code >= 300 {
		t.Fatalf("list expenses: %d", code)
	}
	var expData struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(resp.Data, &expData)
	if expData.Total != 1 {
		t.Fatalf("expenses total = %d, want 1（不得重复入账）", expData.Total)
	}

	// 暂停 / 恢复 / 移除
	if code, _ = do("POST", fmt.Sprintf("/api/v1/plans/%d/pause", plan.ID), token, nil); code >= 300 {
		t.Fatalf("pause: %d", code)
	}
	if code, _ = do("POST", fmt.Sprintf("/api/v1/plans/%d/resume", plan.ID), token, nil); code >= 300 {
		t.Fatalf("resume: %d", code)
	}
	if code, _ = do("DELETE", fmt.Sprintf("/api/v1/plans/%d", plan.ID), token, nil); code >= 300 {
		t.Fatalf("remove: %d", code)
	}
	// 移除后列表为空、消费记录保留
	code, resp = do("GET", fmt.Sprintf("/api/v1/groups/%d/plans", group.ID), token, nil)
	_ = json.Unmarshal(resp.Data, &listData)
	if code >= 300 || listData.Total != 0 {
		t.Fatalf("plans after remove = %d, want 0", listData.Total)
	}
	_, resp = do("GET", fmt.Sprintf("/api/v1/groups/%d/expenses", group.ID), token, nil)
	_ = json.Unmarshal(resp.Data, &expData)
	if expData.Total != 1 {
		t.Fatalf("expenses after remove = %d, want 1（已生成记录保持不变）", expData.Total)
	}
}
