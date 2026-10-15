package shared

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Base struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b *Base) BeforeCreate(*gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

type User struct {
	Base
	ExternalSubject string `gorm:"uniqueIndex;not null"`
	DisplayName     string `gorm:"not null"`
	Email           *string
	Role            string `gorm:"not null;default:user" json:"role"`
	Status          string `gorm:"not null;default:active"`
}
type Course struct {
	Base
	Code *string `gorm:"uniqueIndex"`
	Name string  `gorm:"not null"`
}
type CourseInstance struct {
	Base
	CourseID    uuid.UUID `gorm:"type:uuid;not null"`
	Course      Course
	TermYear    int    `gorm:"not null"`
	TermSeason  string `gorm:"not null"`
	TeacherName *string
	Timezone    string    `gorm:"not null;default:Asia/Shanghai"`
	Status      string    `gorm:"not null;default:active"`
	CreatedBy   uuid.UUID `gorm:"type:uuid;not null"`
}
type Enrollment struct {
	Base
	UserID           uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_enrollment"`
	CourseInstanceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_enrollment"`
	Status           string    `gorm:"not null;default:active"`
	JoinedAt         time.Time `gorm:"not null"`
}
type Assignment struct {
	Base
	CourseInstanceID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"course_instance_id"`
	Title             string     `gorm:"not null" json:"title"`
	Description       string     `gorm:"not null;default:''" json:"description"`
	OfficialDueAt     *time.Time `gorm:"index" json:"official_due_at"`
	SubmissionMethod  *string    `json:"submission_method"`
	Status            string     `gorm:"not null;default:published" json:"status"`
	SourceCandidateID *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"source_candidate_id"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
}
type AssignmentUserState struct {
	Base
	AssignmentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_assignment_state"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_assignment_state"`
	PersonalDueAt    *time.Time
	CompletionStatus string `gorm:"not null;default:todo"`
	CompletedAt      *time.Time
}
type SourceItem struct {
	Base
	CourseInstanceID *uuid.UUID `gorm:"type:uuid;index"`
	SubmittedBy      uuid.UUID  `gorm:"type:uuid;not null"`
	SourceType       string     `gorm:"not null"`
	RawText          *string
	OriginalFilename *string
	MimeType         *string
	ByteSize         *int64
	Checksum         *string
	Status           string `gorm:"not null;default:queued"`
}
type IngestionJob struct {
	Base
	SourceItemID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"source_item_id"`
	Stage            string     `gorm:"not null;default:queued" json:"stage"`
	Status           string     `gorm:"not null;default:queued" json:"status"`
	Attempts         int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts      int        `gorm:"not null;default:3" json:"max_attempts"`
	QueueTaskID      *string    `json:"-"`
	TemporaryLocator *string    `json:"-"`
	ErrorCode        *string    `json:"error_code"`
	ErrorMessage     *string    `json:"error_message"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
}
type AssignmentCandidate struct {
	Base
	SourceItemID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:ux_candidate_ordinal" json:"source_item_id"`
	Ordinal          int        `gorm:"not null;uniqueIndex:ux_candidate_ordinal" json:"ordinal"`
	Title            string     `gorm:"not null" json:"title"`
	Description      string     `gorm:"not null;default:''" json:"description"`
	OfficialDueAt    *time.Time `json:"official_due_at"`
	RawDueExpression *string    `json:"raw_due_expression"`
	SubmissionMethod *string    `json:"submission_method"`
	Evidence         string     `gorm:"type:jsonb;not null;default:'[]'" json:"evidence"`
	FieldConfidence  string     `gorm:"type:jsonb;not null;default:'{}'" json:"field_confidence"`
	UncertainFields  string     `gorm:"type:jsonb;not null;default:'[]'" json:"uncertain_fields"`
	CourseInstanceID *uuid.UUID `gorm:"type:uuid" json:"course_instance_id"`
	ReviewStatus     string     `gorm:"not null;default:pending" json:"review_status"`
	ReviewedBy       *uuid.UUID `gorm:"type:uuid" json:"reviewed_by"`
	ReviewedAt       *time.Time `json:"reviewed_at"`
	ReviewNote       *string    `json:"review_note"`
}
type Notification struct {
	Base
	UserID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Kind    string     `gorm:"not null" json:"kind"`
	Payload string     `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	ReadAt  *time.Time `json:"read_at"`
}

func Models() []any {
	return []any{&User{}, &Course{}, &CourseInstance{}, &Enrollment{}, &Assignment{}, &AssignmentUserState{}, &SourceItem{}, &IngestionJob{}, &AssignmentCandidate{}, &Notification{}}
}
