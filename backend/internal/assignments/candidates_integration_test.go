package assignments

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"homeworkcanvas/backend/internal/courses"
	"homeworkcanvas/backend/internal/shared"
)

func TestAcceptEditedCandidateClearsDueDate(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the isolated PostgreSQL integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	schema := "canvas_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := tx.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("SET LOCAL search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.AutoMigrate(shared.Models()...); err != nil {
		t.Fatal(err)
	}

	user := shared.User{ExternalSubject: "review-user", DisplayName: "Reviewer", Status: "active"}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	member := courses.Service{DB: tx}
	instance, err := member.CreateInstance(context.Background(), user.ID, courses.CreateInstanceInput{CourseName: "Review Course", TermYear: 2026, TermSeason: "秋"})
	if err != nil {
		t.Fatal(err)
	}
	source := shared.SourceItem{CourseInstanceID: &instance.ID, SubmittedBy: user.ID, SourceType: "text", Status: "review_ready"}
	if err := tx.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	job := shared.IngestionJob{SourceItemID: source.ID, Stage: "review_ready", Status: "review_ready", MaxAttempts: 3}
	if err := tx.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(24 * time.Hour)
	candidate := shared.AssignmentCandidate{SourceItemID: source.ID, Ordinal: 1, Title: "模型标题", Description: "模型说明", OfficialDueAt: &due, ReviewStatus: "pending", Evidence: "[]", FieldConfidence: "{}", UncertainFields: "[]"}
	if err := tx.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}

	service := Service{DB: tx, Membership: member}
	assignment, err := service.AcceptCandidate(context.Background(), user.ID, candidate.ID, CreateInput{Edited: true, Title: "人工确认标题", Description: "人工确认说明", OfficialDueAt: nil, SubmissionMethod: nil})
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Title != "人工确认标题" || assignment.OfficialDueAt != nil || assignment.SubmissionMethod != nil {
		t.Fatalf("published assignment ignored review edits: %+v", assignment)
	}
	var reviewed shared.AssignmentCandidate
	if err := tx.First(&reviewed, "id=?", candidate.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reviewed.Title != assignment.Title || reviewed.OfficialDueAt != nil || reviewed.ReviewStatus != "accepted" {
		t.Fatalf("candidate edits or review state missing: %+v", reviewed)
	}
}
