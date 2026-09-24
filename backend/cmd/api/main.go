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
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	_ = shared.LoadDotEnv(".env")
	shared.ConfigureLogging()
	db, e := shared.OpenDatabase()
	if e != nil {
		panic(e)
	}
	queue := asynq.NewClient(shared.RedisOpt())
	cs := courses.Service{DB: db}
	ns := notifications.Service{DB: db}
	as := assignments.Service{
		DB: db, Membership: cs,
		PublishNotification: ns.AssignmentPublished,
		UpdateNotification:  ns.AssignmentUpdated,
		RetractNotification: ns.AssignmentRetracted,
	}
	in := ingestion.Service{DB: db, Membership: cs, Queue: queue, Extractor: ingestion.NewConfiguredExtractor(), Files: shared.NewTemporaryFiles()}
	r := gin.New()
	r.Use(shared.RequestLog(), shared.SafeRecovery(), devCORS())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.OPTIONS("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	api := r.Group("/api/v1")
	api.Use(identity.HeaderIdentity(db))
	api.GET("/me", func(c *gin.Context) {
		u := identity.Current(c)
		c.JSON(200, gin.H{"id": u.ID, "display_name": u.DisplayName, "role": u.Role})
	})
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
		v, e := cs.ListInstances(c, identity.Current(c).ID, false)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/course-instances", func(c *gin.Context) {
		v, e := cs.ListInstances(c, identity.Current(c).ID, true)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.PUT("/course-instances/:id/enrollment", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		e = cs.SetEnrollment(c, identity.Current(c).ID, ci, true)
		respond(c, gin.H{"joined": true}, e, 200)
	})
	api.DELETE("/course-instances/:id/enrollment", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		e = cs.SetEnrollment(c, identity.Current(c).ID, ci, false)
		respond(c, gin.H{"joined": false}, e, 200)
	})
	api.GET("/admin/course-instances", func(c *gin.Context) {
		v, e := cs.ListAllInstances(c, identity.Current(c).ID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.PATCH("/course-instances/:id", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x courses.UpdateInstanceInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := cs.UpdateInstance(c, identity.Current(c).ID, ci, x)
		respond(c, v, e, 200)
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
	api.GET("/course-instances/:id/meetings", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := cs.ListMeetings(c, identity.Current(c).ID, ci)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/schedule", func(c *gin.Context) {
		v, e := cs.ListSchedule(c, identity.Current(c).ID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/assignments", func(c *gin.Context) {
		courseID, e := optionalCourseID(c)
		if e != nil {
			shared.RespondError(c, e)
			return
		}
		v, e := as.ListMine(c, identity.Current(c).ID, c.DefaultQuery("bucket", "all"), courseID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/admin/assignments", func(c *gin.Context) {
		courseID, limit, e := listParams(c)
		if e != nil {
			shared.RespondError(c, e)
			return
		}
		v, next, e := as.ListAdmin(c, identity.Current(c).ID, c.DefaultQuery("status", "published"), courseID, limit, c.Query("cursor"))
		respond(c, gin.H{"data": v, "next_cursor": next}, e, 200)
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
	api.PATCH("/assignments/:id", func(c *gin.Context) {
		aid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		var x assignments.CreateInput
		if c.ShouldBindJSON(&x) != nil {
			bad(c)
			return
		}
		v, e := as.UpdatePublished(c, identity.Current(c).ID, aid, x)
		respond(c, v, e, 200)
	})
	api.POST("/assignments/:id/retract", func(c *gin.Context) {
		aid, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := as.Retract(c, identity.Current(c).ID, aid)
		respond(c, v, e, 200)
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
		result, e := in.CreateFileSource(c, identity.Current(c).ID, ci, file, c.PostForm("text"))
		status := http.StatusAccepted
		if result.Reused {
			status = http.StatusOK
		}
		respond(c, gin.H{"source": result.Source, "job": result.Job, "reused": result.Reused}, e, status)
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
			if e = cs.RequireAdmin(c, identity.Current(c).ID); e != nil {
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
		if e = cs.RequireAdmin(c, identity.Current(c).ID); e != nil {
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
	api.GET("/admin/assignment-candidates", func(c *gin.Context) {
		courseID, limit, e := listParams(c)
		if e != nil {
			shared.RespondError(c, e)
			return
		}
		v, next, e := as.ListPending(c, identity.Current(c).ID, courseID, limit, c.Query("cursor"))
		respond(c, gin.H{"data": v, "next_cursor": next}, e, 200)
	})
	api.GET("/admin/source-issues", func(c *gin.Context) {
		courseID, limit, e := listParams(c)
		if e != nil {
			shared.RespondError(c, e)
			return
		}
		v, next, e := in.ListAdminIssues(c, identity.Current(c).ID, courseID, limit, c.Query("cursor"))
		respond(c, gin.H{"data": v, "next_cursor": next}, e, 200)
	})
	api.GET("/course-instances/:id/assignments", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := as.ListCourse(c, identity.Current(c).ID, ci)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/me/sources", func(c *gin.Context) {
		v, e := in.ListMine(c, identity.Current(c).ID)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	api.GET("/course-instances/:id/sources", func(c *gin.Context) {
		ci, e := id(c)
		if e != nil {
			bad(c)
			return
		}
		v, e := in.ListCourseSources(c, identity.Current(c).ID, ci)
		respond(c, gin.H{"data": v, "next_cursor": nil}, e, 200)
	})
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	if e := srv.ListenAndServe(); e != nil {
		log.Fatal(e)
	}
}

func devCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Expose-Headers", "X-Request-ID")
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
func optionalCourseID(c *gin.Context) (*uuid.UUID, error) {
	raw := c.Query("course_instance_id")
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, shared.JSONError("shared.invalid_course", "课程筛选无效", 400)
	}
	return &id, nil
}
func listParams(c *gin.Context) (*uuid.UUID, int, error) {
	id, err := optionalCourseID(c)
	if err != nil {
		return nil, 0, err
	}
	limit, err := shared.PageLimit(c.Query("limit"))
	return id, limit, err
}
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
