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
	Term        string `gorm:"not null"`
	TeacherName *string
	Timezone    string    `gorm:"not null;default:Asia/Shanghai"`
	Status      string    `gorm:"not null;default:active"`
	CreatedBy   uuid.UUID `gorm:"type:uuid;not null"`
}
type Enrollment struct {
	Base
	UserID           uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_enrollment"`
	CourseInstanceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_enrollment"`
	Role             string    `gorm:"not null"`
	Status           string    `gorm:"not null;default:active"`
	JoinedAt         time.Time `gorm:"not null"`
}
type CourseMeeting struct {
	Base
	CourseInstanceID uuid.UUID  `gorm:"type:uuid;not null;index" json:"course_instance_id"`
	Weekday          int        `gorm:"not null" json:"weekday"`
	StartTime        string     `gorm:"not null" json:"start_time"`
	EndTime          string     `gorm:"not null" json:"end_time"`
	Location         *string    `json:"location"`
	ValidFrom        *time.Time `gorm:"type:date" json:"valid_from"`
	ValidTo          *time.Time `gorm:"type:date" json:"valid_to"`
}
type Assignment struct {
	Base
	CourseInstanceID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	Title             string     `gorm:"not null"`
	Description       string     `gorm:"not null;default:''"`
	OfficialDueAt     *time.Time `gorm:"index"`
	SubmissionMethod  *string
	Status            string     `gorm:"not null;default:published"`
	SourceCandidateID *uuid.UUID `gorm:"type:uuid;uniqueIndex"`
	CreatedBy         uuid.UUID  `gorm:"type:uuid;not null"`
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
	CourseInstanceID uuid.UUID `gorm:"type:uuid;not null;index"`
	SubmittedBy      uuid.UUID `gorm:"type:uuid;not null"`
	SourceType       string    `gorm:"not null"`
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
	SourceItemID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_candidate_ordinal"`
	Ordinal          int       `gorm:"not null;uniqueIndex:ux_candidate_ordinal"`
	Title            string    `gorm:"not null"`
	Description      string    `gorm:"not null;default:''"`
	OfficialDueAt    *time.Time
	RawDueExpression *string
	SubmissionMethod *string
	Evidence         string     `gorm:"type:jsonb;not null;default:'[]'"`
	FieldConfidence  string     `gorm:"type:jsonb;not null;default:'{}'"`
	UncertainFields  string     `gorm:"type:jsonb;not null;default:'[]'"`
	ReviewStatus     string     `gorm:"not null;default:pending"`
	ReviewedBy       *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt       *time.Time
	ReviewNote       *string
}
type Notification struct {
	Base
	UserID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Kind    string     `gorm:"not null" json:"kind"`
	Payload string     `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	ReadAt  *time.Time `json:"read_at"`
}

func Models() []any {
	return []any{&User{}, &Course{}, &CourseInstance{}, &Enrollment{}, &CourseMeeting{}, &Assignment{}, &AssignmentUserState{}, &SourceItem{}, &IngestionJob{}, &AssignmentCandidate{}, &Notification{}}
}
