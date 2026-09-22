package courses

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"homeworkcanvas/backend/internal/shared"
	"time"
)

type Service struct{ DB *gorm.DB }

func (s Service) RequireActiveMember(ctx context.Context, u, ci uuid.UUID) error {
	var n int64
	s.DB.WithContext(ctx).Model(&shared.Enrollment{}).Where("user_id=? AND course_instance_id=? AND status='active'", u, ci).Count(&n)
	if n == 0 {
		return shared.JSONError("courses.not_member", "你不是该课程的有效成员", 403)
	}
	return nil
}
func (s Service) RequireMaintainer(ctx context.Context, u, ci uuid.UUID) error {
	var n int64
	s.DB.WithContext(ctx).Model(&shared.Enrollment{}).Where("user_id=? AND course_instance_id=? AND status='active' AND role='maintainer'", u, ci).Count(&n)
	if n == 0 {
		return shared.JSONError("courses.not_maintainer", "你没有维护该课程的权限", 403)
	}
	return nil
}

type CreateInstanceInput struct {
	CourseID    *uuid.UUID `json:"course_id"`
	CourseName  string     `json:"course_name"`
	CourseCode  *string    `json:"course_code"`
	Term        string     `json:"term"`
	TeacherName *string    `json:"teacher_name"`
	Timezone    string     `json:"timezone"`
}

func (s Service) CreateInstance(ctx context.Context, u uuid.UUID, in CreateInstanceInput) (shared.CourseInstance, error) {
	if in.Term == "" || (in.CourseID == nil && in.CourseName == "") {
		return shared.CourseInstance{}, shared.JSONError("courses.invalid_input", "课程名称和学期不能为空", 400)
	}
	tz := in.Timezone
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	var out shared.CourseInstance
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cid uuid.UUID
		if in.CourseID != nil {
			var c shared.Course
			if e := tx.First(&c, "id=?", *in.CourseID).Error; e != nil {
				return shared.JSONError("courses.not_found", "课程不存在", 404)
			}
			cid = c.ID
		} else {
			c := shared.Course{Name: in.CourseName, Code: in.CourseCode}
			if e := tx.Create(&c).Error; e != nil {
				return e
			}
			cid = c.ID
		}
		out = shared.CourseInstance{CourseID: cid, Term: in.Term, TeacherName: in.TeacherName, Timezone: tz, Status: "active", CreatedBy: u}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Create(&shared.Enrollment{UserID: u, CourseInstanceID: out.ID, Role: "maintainer", Status: "active", JoinedAt: time.Now()}).Error
	})
	return out, e
}
func (s Service) PutMeetings(ctx context.Context, u, ci uuid.UUID, items []shared.CourseMeeting) ([]shared.CourseMeeting, error) {
	if e := s.RequireMaintainer(ctx, u, ci); e != nil {
		return nil, e
	}
	for i := range items {
		if items[i].Weekday < 1 || items[i].Weekday > 7 || items[i].StartTime >= items[i].EndTime {
			return nil, shared.JSONError("courses.invalid_meeting", "上课时间不合法", 400)
		}
		items[i].CourseInstanceID = ci
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("course_instance_id=?", ci).Delete(&shared.CourseMeeting{}).Error; e != nil {
			return e
		}
		for i := range items {
			if e := tx.Create(&items[i]).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return items, e
}

type UpdateEnrollmentInput struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

func (s Service) UpdateEnrollment(ctx context.Context, actor, ci, enrollmentID uuid.UUID, in UpdateEnrollmentInput) (shared.Enrollment, error) {
	if e := s.RequireMaintainer(ctx, actor, ci); e != nil {
		return shared.Enrollment{}, e
	}
	var target shared.Enrollment
	if e := s.DB.WithContext(ctx).First(&target, "id=? AND course_instance_id=?", enrollmentID, ci).Error; e != nil {
		return shared.Enrollment{}, shared.JSONError("courses.enrollment_not_found", "课程成员不存在", 404)
	}
	if in.Role == nil && in.Status == nil {
		return shared.Enrollment{}, shared.JSONError("courses.invalid_enrollment", "没有可更新的字段", 400)
	}
	if in.Role != nil && *in.Role != "maintainer" && *in.Role != "student" {
		return shared.Enrollment{}, shared.JSONError("courses.invalid_enrollment", "角色不合法", 400)
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "left" {
		return shared.Enrollment{}, shared.JSONError("courses.invalid_enrollment", "成员状态不合法", 400)
	}
	leavesMaintainer := target.Role == "maintainer" && ((in.Role != nil && *in.Role != "maintainer") || (in.Status != nil && *in.Status != "active"))
	if leavesMaintainer {
		var count int64
		if e := s.DB.WithContext(ctx).Model(&shared.Enrollment{}).Where("course_instance_id=? AND status='active' AND role='maintainer'", ci).Count(&count).Error; e != nil {
			return shared.Enrollment{}, e
		}
		if count <= 1 {
			return shared.Enrollment{}, shared.JSONError("courses.last_maintainer", "课程至少需要保留一名维护者", 409)
		}
	}
	if in.Role != nil {
		target.Role = *in.Role
	}
	if in.Status != nil {
		target.Status = *in.Status
	}
	return target, s.DB.WithContext(ctx).Save(&target).Error
}

type EnrollmentItem struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	JoinedAt    time.Time `json:"joined_at"`
}

func (s Service) ListEnrollments(ctx context.Context, actor, ci uuid.UUID) ([]EnrollmentItem, error) {
	if e := s.RequireMaintainer(ctx, actor, ci); e != nil {
		return nil, e
	}
	var rows []EnrollmentItem
	e := s.DB.WithContext(ctx).Table("enrollments e").Select("e.id,e.user_id,u.display_name,e.role,e.status,e.joined_at").Joins("JOIN users u ON u.id=e.user_id").Where("e.course_instance_id=?", ci).Order("e.role DESC,e.joined_at ASC").Scan(&rows).Error
	return rows, e
}

type ScheduleItem struct {
	CourseInstanceID uuid.UUID  `json:"course_instance_id"`
	CourseName       string     `json:"course_name"`
	Term             string     `json:"term"`
	Weekday          int        `json:"weekday"`
	StartTime        string     `json:"start_time"`
	EndTime          string     `json:"end_time"`
	Location         *string    `json:"location"`
	ValidFrom        *time.Time `json:"valid_from"`
	ValidTo          *time.Time `json:"valid_to"`
}

func (s Service) ListSchedule(ctx context.Context, u uuid.UUID) ([]ScheduleItem, error) {
	var rows []ScheduleItem
	e := s.DB.WithContext(ctx).Table("enrollments e").Select("ci.id course_instance_id,c.name course_name,ci.term,m.weekday,m.start_time,m.end_time,m.location,m.valid_from,m.valid_to").Joins("JOIN course_instances ci ON ci.id=e.course_instance_id").Joins("JOIN courses c ON c.id=ci.course_id").Joins("JOIN course_meetings m ON m.course_instance_id=ci.id").Where("e.user_id=? AND e.status='active' AND ci.status='active'", u).Order("m.weekday,m.start_time,ci.id").Scan(&rows).Error
	return rows, e
}
