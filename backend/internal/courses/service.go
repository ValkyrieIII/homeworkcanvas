package courses

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"homeworkcanvas/backend/internal/shared"
)

type Service struct{ DB *gorm.DB }

func (s Service) RequireActiveMember(ctx context.Context, u, ci uuid.UUID) error {
	var n int64
	if err := s.DB.WithContext(ctx).Table("enrollments e").
		Joins("JOIN course_instances ci ON ci.id=e.course_instance_id").
		Where("e.user_id=? AND e.course_instance_id=? AND e.status='active' AND ci.status='active'", u, ci).
		Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return shared.JSONError("courses.not_member", "你不是该课程的有效成员", 403)
	}
	return nil
}
func (s Service) RequireAdmin(ctx context.Context, u uuid.UUID) error {
	var n int64
	if err := s.DB.WithContext(ctx).Model(&shared.User{}).Where("id=? AND role='admin' AND status='active'", u).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return shared.JSONError("courses.not_admin", "需要管理员身份", 403)
	}
	return nil
}

type InstanceItem struct {
	ID          uuid.UUID `json:"id"`
	CourseName  string    `json:"course_name"`
	Term        string    `json:"term"`
	TeacherName *string   `json:"teacher_name"`
	Timezone    string    `json:"timezone"`
	Status      string    `json:"status"`
	Joined      bool      `json:"joined"`
}

func (s Service) ListInstances(ctx context.Context, user uuid.UUID, onlyMine bool) ([]InstanceItem, error) {
	q := s.DB.WithContext(ctx).Table("course_instances ci").
		Select("ci.id,c.name AS course_name,ci.term,ci.teacher_name,ci.timezone,ci.status,(e.id IS NOT NULL) AS joined").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Joins("LEFT JOIN enrollments e ON e.course_instance_id=ci.id AND e.user_id=? AND e.status='active'", user)
	if onlyMine {
		q = q.Where("e.id IS NOT NULL AND ci.status='active'")
	} else {
		q = q.Where("ci.status='active'")
	}
	var rows []InstanceItem
	err := q.Order("ci.term DESC,c.name ASC,ci.id ASC").Scan(&rows).Error
	return rows, err
}

func (s Service) ListAllInstances(ctx context.Context, actor uuid.UUID) ([]InstanceItem, error) {
	if err := s.RequireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	var rows []InstanceItem
	err := s.DB.WithContext(ctx).Table("course_instances ci").
		Select("ci.id,c.name AS course_name,ci.term,ci.teacher_name,ci.timezone,ci.status,(e.id IS NOT NULL) AS joined").
		Joins("JOIN courses c ON c.id=ci.course_id").
		Joins("LEFT JOIN enrollments e ON e.course_instance_id=ci.id AND e.user_id=? AND e.status='active'", actor).
		Order("ci.term DESC,c.name ASC,ci.id ASC").Scan(&rows).Error
	return rows, err
}

// SetEnrollment 只改当前用户的参与关系，重返课程时复用原记录。
func (s Service) SetEnrollment(ctx context.Context, user, ci uuid.UUID, joined bool) error {
	var instance shared.CourseInstance
	if err := s.DB.WithContext(ctx).First(&instance, "id=? AND status='active'", ci).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return shared.JSONError("courses.not_found", "课程不存在", 404)
		}
		return err
	}
	if joined {
		row := shared.Enrollment{UserID: user, CourseInstanceID: ci, Status: "active", JoinedAt: time.Now()}
		return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "course_instance_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "joined_at", "updated_at"}),
		}).Create(&row).Error
	}
	return s.DB.WithContext(ctx).Model(&shared.Enrollment{}).
		Where("user_id=? AND course_instance_id=?", user, ci).Update("status", "left").Error
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
	if err := s.RequireAdmin(ctx, u); err != nil {
		return shared.CourseInstance{}, err
	}
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
		return nil
	})
	return out, e
}
func (s Service) PutMeetings(ctx context.Context, u, ci uuid.UUID, items []shared.CourseMeeting) ([]shared.CourseMeeting, error) {
	if e := s.RequireAdmin(ctx, u); e != nil {
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

type EnrollmentItem struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	JoinedAt    time.Time `json:"joined_at"`
}

func (s Service) ListEnrollments(ctx context.Context, actor, ci uuid.UUID) ([]EnrollmentItem, error) {
	if e := s.RequireAdmin(ctx, actor); e != nil {
		return nil, e
	}
	var rows []EnrollmentItem
	e := s.DB.WithContext(ctx).Table("enrollments e").
		Select("e.id,e.user_id,u.display_name,e.status,e.joined_at").
		Joins("JOIN users u ON u.id=e.user_id").
		Where("e.course_instance_id=?", ci).Order("e.joined_at ASC").Scan(&rows).Error
	return rows, e
}

type UpdateInstanceInput struct {
	CourseName  string  `json:"course_name"`
	Term        string  `json:"term"`
	TeacherName *string `json:"teacher_name"`
	Timezone    string  `json:"timezone"`
	Status      string  `json:"status"`
}

func (s Service) UpdateInstance(ctx context.Context, actor, ci uuid.UUID, in UpdateInstanceInput) (shared.CourseInstance, error) {
	if err := s.RequireAdmin(ctx, actor); err != nil {
		return shared.CourseInstance{}, err
	}
	if strings.TrimSpace(in.CourseName) == "" || strings.TrimSpace(in.Term) == "" || (in.Status != "active" && in.Status != "archived") {
		return shared.CourseInstance{}, shared.JSONError("courses.invalid_input", "课程名称、学期或状态不合法", 400)
	}
	var instance shared.CourseInstance
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&instance, "id=?", ci).Error; err != nil {
			return shared.JSONError("courses.not_found", "课程不存在", 404)
		}
		if err := tx.Model(&shared.Course{}).Where("id=?", instance.CourseID).Update("name", strings.TrimSpace(in.CourseName)).Error; err != nil {
			return err
		}
		instance.Term, instance.TeacherName, instance.Status = strings.TrimSpace(in.Term), in.TeacherName, in.Status
		if in.Timezone != "" {
			instance.Timezone = in.Timezone
		}
		return tx.Save(&instance).Error
	})
	return instance, err
}

func (s Service) ListMeetings(ctx context.Context, actor, ci uuid.UUID) ([]shared.CourseMeeting, error) {
	if err := s.RequireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	var rows []shared.CourseMeeting
	err := s.DB.WithContext(ctx).Where("course_instance_id=?", ci).Order("weekday,start_time,id").Find(&rows).Error
	return rows, err
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
