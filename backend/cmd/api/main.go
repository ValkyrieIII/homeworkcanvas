package main

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"homeworkcanvas/backend/internal/assignments"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/identity"
	"homeworkcanvas/backend/internal/ingestion"
	"homeworkcanvas/backend/internal/notifications"
	"homeworkcanvas/backend/internal/shared"
	"net/http"
	"os"
)

func main() {
	db, e := shared.OpenDatabase()
	if e != nil {
		panic(e)
	}
	queue := asynq.NewClient(shared.RedisOpt())
	cs := courses.Service{DB: db}
	ns := notifications.Service{DB: db}
	as := assignments.Service{DB: db, Membership: cs, PublishNotification: ns.AssignmentPublished}
	in := ingestion.Service{DB: db, Membership: cs, Queue: queue, Extractor: ingestion.NewConfiguredExtractor(), Files: shared.NewTemporaryFiles()}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), devCORS())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.OPTIONS("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	api := r.Group("/api/v1")
	api.Use(identity.HeaderIdentity(db))
	api.POST("/course-instances", func(c *gin.Context) {
		var x courses.CreateInstanceInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := cs.CreateInstance(c, identity.Current(c).ID, x)
		respond(c, v, e, 201)
	})
	api.GET("/course-instances", func(c *gin.Context) {
		u := identity.Current(c)
		type courseItem struct {
			ID          uuid.UUID `json:"id"`
			CourseName  string    `json:"course_name"`
			Term        string    `json:"term"`
			TeacherName *string   `json:"teacher_name"`
			Timezone    string    `json:"timezone"`
			MyRole      string    `json:"my_role"`
		}
		var rows []courseItem
		e := db.Table("course_instances ci").Select("ci.id,c.name AS course_name,ci.term,ci.teacher_name,ci.timezone,e.role AS my_role").Joins("JOIN courses c ON c.id=ci.course_id").Joins("JOIN enrollments e ON e.course_instance_id=ci.id").Where("e.user_id=? AND e.status='active'", u.ID).Order("ci.term DESC,c.name ASC").Scan(&rows).Error
		respond(c, gin.H{"data": rows, "next_cursor": nil}, e, 200)
	})
	api.POST("/course-instances/:id/enrollments", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		if e = cs.RequireMaintainer(c, identity.Current(c).ID, ci); e != nil {
			shared.RespondError(c, e)
			return
		}
		var x struct {
			UserID uuid.UUID `json:"user_id"`
			Role   string    `json:"role"`
		}
		if c.ShouldBindJSON(&x) != nil || x.UserID == uuid.Nil {
			bad(c)
			return
		}
		if x.Role == "" {
			x.Role = "student"
		}
		row := shared.Enrollment{UserID: x.UserID, CourseInstanceID: ci, Role: x.Role, Status: "active"}
		e = db.Create(&row).Error
		respond(c, row, e, 201)
	})
	api.GET("/course-instances/:id/enrollments", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := cs.ListEnrollments(c, identity.Current(c).ID, ci)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.PATCH("/course-instances/:id/enrollments/:enrollmentId", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		enrollmentID, e := uuid.Parse(c.Param("enrollmentId"))
		if e != nil {
			bad(c)
			return
		}
		var x courses.UpdateEnrollmentInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := cs.UpdateEnrollment(c, identity.Current(c).ID, ci, enrollmentID, x)
		respond(c, v, e, 200)
	})
	api.PUT("/course-instances/:id/meetings", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x []shared.CourseMeeting
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := cs.PutMeetings(c, identity.Current(c).ID, ci, x)
		respond(c, v, e, 200)
	})
	api.GET("/me/schedule", func(c *gin.Context) {
		v, e := cs.ListSchedule(c, identity.Current(c).ID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/assignments", func(c *gin.Context) {
		v, e := as.ListMine(c, identity.Current(c).ID, c.DefaultQuery("bucket", "all"))
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/notifications", func(c *gin.Context) {
		v, e := ns.ListMine(c, identity.Current(c).ID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.POST("/notifications/:id/read", func(c *gin.Context) {
		nid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		e = ns.MarkRead(c, identity.Current(c).ID, nid)
		respond(c, gin.H{"id": nid, "read": true}, e, 200)
	})
	api.POST("/course-instances/:id/assignments", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x assignments.CreateInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := as.CreateManual(c, identity.Current(c).ID, ci, x)
		respond(c, v, e, 201)
	})
	api.PUT("/assignments/:id/my-state", func(c *gin.Context) {
		aid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x assignments.StateInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := as.UpdateMyState(c, identity.Current(c).ID, aid, x)
		respond(c, v, e, 200)
	})
	api.POST("/course-instances/:id/sources/text", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x struct {
			Text string `json:"text"`
		}
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		s, j, e := in.CreateTextSource(c, identity.Current(c).ID, ci, x.Text)
		respond(c, gin.H{"source": s, "job": j}, e, 202)
	})
	api.POST("/course-instances/:id/sources/file", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		file, e := c.FormFile("file")
		if e != nil {
			bad(c)
			return
		}
		s, j, e := in.CreateFileSource(c, identity.Current(c).ID, ci, file)
		respond(c, gin.H{"source": s, "job": j}, e, 202)
	})
	api.GET("/ingestion-jobs/:id", func(c *gin.Context) {
		jid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var j shared.IngestionJob
		if e = db.First(&j, "id=?", jid).Error; e != nil {
			shared.RespondError(c, shared.JSONError("ingestion.not_found", "任务不存在", 404))
			return
		}
		var s shared.SourceItem
		db.First(&s, "id=?", j.SourceItemID)
		if s.SubmittedBy != identity.Current(c).ID {
			if e = cs.RequireMaintainer(c, identity.Current(c).ID, s.CourseInstanceID); e != nil {
				shared.RespondError(c, e)
				return
			}
		}
		respond(c, j, nil, 200)
	})
	api.POST("/assignment-candidates/:id/accept", func(c *gin.Context) {
		cid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x assignments.CreateInput
		c.ShouldBindJSON(&x)
		v, e := as.AcceptCandidate(c, identity.Current(c).ID, cid, x)
		respond(c, v, e, 201)
	})
	api.POST("/assignment-candidates/:id/reject", func(c *gin.Context) {
		cid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x struct {
			Note *string `json:"note"`
		}
		c.ShouldBindJSON(&x)
		e = as.RejectCandidate(c, identity.Current(c).ID, cid, x.Note)
		respond(c, gin.H{"id": cid, "review_status": "rejected"}, e, 200)
	})
	api.GET("/course-instances/:id/assignment-candidates", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		if e = cs.RequireMaintainer(c, identity.Current(c).ID, ci); e != nil {
			shared.RespondError(c, e)
			return
		}
		status := c.DefaultQuery("status", "pending")
		var rows []shared.AssignmentCandidate
		e = db.Table("assignment_candidates ac").Joins("JOIN source_items s ON s.id=ac.source_item_id").Where("s.course_instance_id=? AND ac.review_status=?", ci, status).Order("ac.created_at ASC").Find(&rows).Error
		respond(c, gin.H{"data": rows, "next_cursor": nil}, e, 200)
	})
	api.GET("/course-instances/:id/ingestion-metrics", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := in.ReviewMetrics(c, identity.Current(c).ID, ci)
		respond(c, v, e, 200)
	})
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_ = http.ListenAndServe(addr, r)
}

func devCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Canvas-Subject, X-Canvas-Name, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.Status(204)
			c.Abort()
			return
		}
		c.Next()
	}
}
func id(c *gin.Context) (uuid.UUID, error) { return uuid.Parse(c.Param("id")) }
func bad(c *gin.Context) {
	shared.RespondError(c, shared.JSONError("shared.invalid_request", "请求参数不合法", 400))
}
func respond(c *gin.Context, v any, e error, status int) {
	if e != nil {
		shared.RespondError(c, e)
		return
	}
	c.JSON(status, v)
}
