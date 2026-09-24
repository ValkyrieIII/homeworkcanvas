package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"homeworkcanvas/backend/internal/shared"
)

type fixedExtractor []Extracted

func (f fixedExtractor) Extract(context.Context, ExtractionInput) ([]Extracted, error) {
	return f, nil
}

type captureExtractor struct{ pages []Page }

func (c *captureExtractor) Extract(ctx context.Context, in ExtractionInput) ([]Extracted, error) {
	c.pages = in.Pages
	return (RuleExtractor{}).Extract(ctx, in)
}

func smallPDF() []byte {
	content := "BT /F1 12 Tf 72 720 Td (Build a simple application and write a report.) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes()
}

// All tables live in a schema created inside one transaction. Rollback removes
// both schema and records, so this never changes the configured database's data.
func TestProcessStateTransitions(t *testing.T) {
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

	user := shared.User{ExternalSubject: "integration-user", DisplayName: "Integration", Status: "active"}
	course := shared.Course{Name: "Integration Course"}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Create(&course).Error; err != nil {
		t.Fatal(err)
	}
	instance := shared.CourseInstance{CourseID: course.ID, Term: "2026-fall", Timezone: "Asia/Shanghai", Status: "active", CreatedBy: user.ID}
	if err := tx.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}

	makeJob := func(text string) (shared.SourceItem, shared.IngestionJob) {
		t.Helper()
		src := shared.SourceItem{CourseInstanceID: instance.ID, SubmittedBy: user.ID, SourceType: "text", RawText: &text, Status: "queued"}
		if err := tx.Create(&src).Error; err != nil {
			t.Fatal(err)
		}
		job := shared.IngestionJob{SourceItemID: src.ID, Stage: "queued", Status: "queued", MaxAttempts: 3}
		if err := tx.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		return src, job
	}

	src, job := makeJob("实验一报告 10月15日提交")
	svc := Service{DB: tx, Extractor: RuleExtractor{}, Files: shared.TemporaryFiles{Root: t.TempDir()}}
	if err := svc.Process(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Model(&shared.AssignmentCandidate{}).Where("source_item_id=?", src.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&job, "id=?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&src, "id=?", src.ID).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || job.Status != "review_ready" || src.Status != "review_ready" || job.Attempts != 1 {
		t.Fatalf("success state: candidates=%d job=%s source=%s attempts=%d", count, job.Status, src.Status, job.Attempts)
	}

	message, filename := "Teacher message: the report is due Friday.", "homework.pdf"
	fileSource := shared.SourceItem{CourseInstanceID: instance.ID, SubmittedBy: user.ID, SourceType: "file", RawText: &message, OriginalFilename: &filename, Status: "queued"}
	if err := tx.Create(&fileSource).Error; err != nil {
		t.Fatal(err)
	}
	fileJob := shared.IngestionJob{Base: shared.Base{ID: uuid.New()}, SourceItemID: fileSource.ID, Stage: "queued", Status: "queued", MaxAttempts: 3}
	dir, err := svc.Files.CreateJobDir(fileJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	fileJob.TemporaryLocator = &dir
	if err := tx.Create(&fileJob).Error; err != nil {
		t.Fatal(err)
	}
	path, err := svc.Files.SourcePath(fileJob.ID, filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, smallPDF(), 0o600); err != nil {
		t.Fatal(err)
	}
	captured := &captureExtractor{}
	svc.Extractor = captured
	if err := svc.Process(context.Background(), fileJob.ID); err != nil {
		t.Fatal(err)
	}
	if len(captured.pages) != 2 || captured.pages[0].Number != 0 || captured.pages[0].Text != message || captured.pages[1].Number != 1 || !strings.Contains(captured.pages[1].Text, "Build a simple application") {
		t.Fatalf("extractor did not receive teacher message and PDF together: %+v", captured.pages)
	}

	badSource, badJob := makeJob("作业截止明天")
	svc.Extractor = fixedExtractor{{Title: "作业", Evidence: []Evidence{{PageNumber: 1, TextExcerpt: "invented evidence"}}}}
	if err := svc.Process(context.Background(), badJob.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&badJob, "id=?", badJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.First(&badSource, "id=?", badSource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if badJob.Status != "failed" || badSource.Status != "failed" || badJob.ErrorCode == nil || *badJob.ErrorCode != "ingestion.invalid_result" {
		t.Fatalf("failure state: job=%s source=%s code=%v", badJob.Status, badSource.Status, badJob.ErrorCode)
	}

	checksum := "same-content-checksum"
	newFile := func(submittedBy uuid.UUID) (shared.SourceItem, shared.IngestionJob) {
		return shared.SourceItem{CourseInstanceID: instance.ID, SubmittedBy: submittedBy, SourceType: "file", Checksum: &checksum, Status: "queued"},
			shared.IngestionJob{Base: shared.Base{ID: uuid.New()}, Stage: "queued", Status: "queued", MaxAttempts: 3}
	}
	firstSource, firstJob := newFile(user.ID)
	if reused, err := svc.createOrReuseFileSource(context.Background(), &firstSource, &firstJob); err != nil || reused {
		t.Fatalf("first file reused=%v err=%v", reused, err)
	}
	secondSource, secondJob := newFile(user.ID)
	if reused, err := svc.createOrReuseFileSource(context.Background(), &secondSource, &secondJob); err != nil || !reused {
		t.Fatalf("duplicate file reused=%v err=%v", reused, err)
	}
	if secondSource.ID != firstSource.ID || secondJob.ID != firstJob.ID {
		t.Fatalf("duplicate created new records: source=%s job=%s", secondSource.ID, secondJob.ID)
	}
	if err := tx.Model(&firstSource).Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	retrySource, retryJob := newFile(user.ID)
	if reused, err := svc.createOrReuseFileSource(context.Background(), &retrySource, &retryJob); err != nil || reused || retryJob.ID == firstJob.ID {
		t.Fatalf("failed file retry reused=%v job=%s err=%v", reused, retryJob.ID, err)
	}
	otherUser := shared.User{ExternalSubject: "other-user", DisplayName: "Other", Status: "active"}
	if err := tx.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	otherSource, otherJob := newFile(otherUser.ID)
	if reused, err := svc.createOrReuseFileSource(context.Background(), &otherSource, &otherJob); err != nil || reused || otherJob.ID == retryJob.ID {
		t.Fatalf("different user reused=%v job=%s err=%v", reused, otherJob.ID, err)
	}
}
