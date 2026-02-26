package domain

// 文件： internal/domain/models.go
// 用途：领域实体与跨层共享的数据模型定义。

import "time"

type User struct {
	ID           string    `gorm:"column:id;primaryKey"`
	Email        string    `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	Name         string    `gorm:"column:name"`
	Phone        *string   `gorm:"column:phone"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string { return "users" }

type Patient struct {
	ID          string    `gorm:"column:id;primaryKey"`
	UserID      string    `gorm:"column:user_id"`
	DateOfBirth time.Time `gorm:"column:date_of_birth"`
	Gender      string    `gorm:"column:gender"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`

	User User `gorm:"foreignKey:UserID;references:ID"`
}

func (Patient) TableName() string { return "patients" }

type Image struct {
	ID           string    `gorm:"column:id;primaryKey"`
	PatientID    string    `gorm:"column:patient_id"`
	FilePath     string    `gorm:"column:file_path"`
	FileType     string    `gorm:"column:file_type"`
	OriginalName string    `gorm:"column:original_name"`
	FileSize     int       `gorm:"column:file_size"`
	Status       string    `gorm:"column:status"`
	UploadedBy   string    `gorm:"column:uploaded_by"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`

	Patient    Patient     `gorm:"foreignKey:PatientID;references:ID"`
	Uploader   User        `gorm:"foreignKey:UploadedBy;references:ID"`
	Detections []Detection `gorm:"foreignKey:ImageID;references:ID"`
}

func (Image) TableName() string { return "images" }

type Detection struct {
	ID                string    `gorm:"column:id;primaryKey"`
	ImageID           string    `gorm:"column:image_id"`
	ModelVersion      string    `gorm:"column:model_version"`
	CancerProbability float64   `gorm:"column:cancer_probability"`
	Findings          []byte    `gorm:"column:findings"`
	HeatmapPath       *string   `gorm:"column:heatmap_path"`
	Status            string    `gorm:"column:status"`
	ReviewedBy        *string   `gorm:"column:reviewed_by"`
	ReviewNotes       *string   `gorm:"column:review_notes"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`

	Image    Image    `gorm:"foreignKey:ImageID;references:ID"`
	Reviewer *User    `gorm:"foreignKey:ReviewedBy;references:ID"`
	Reports  []Report `gorm:"foreignKey:DetectionID;references:ID"`
}

func (Detection) TableName() string { return "detections" }

type Report struct {
	ID          string    `gorm:"column:id;primaryKey"`
	DetectionID string    `gorm:"column:detection_id"`
	PatientID   string    `gorm:"column:patient_id"`
	DoctorID    string    `gorm:"column:doctor_id"`
	Content     []byte    `gorm:"column:content"`
	PDFPath     *string   `gorm:"column:pdf_path"`
	Status      string    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`

	Detection Detection `gorm:"foreignKey:DetectionID;references:ID"`
	Patient   Patient   `gorm:"foreignKey:PatientID;references:ID"`
	Doctor    User      `gorm:"foreignKey:DoctorID;references:ID"`
}

func (Report) TableName() string { return "reports" }

type AuditLog struct {
	ID          string    `gorm:"column:id;primaryKey"`
	ActorUserID *string   `gorm:"column:actor_user_id"`
	ActorRole   *string   `gorm:"column:actor_role"`
	Action      string    `gorm:"column:action"`
	EntityType  string    `gorm:"column:entity_type"`
	EntityID    string    `gorm:"column:entity_id"`
	Result      string    `gorm:"column:result"`
	Metadata    []byte    `gorm:"column:metadata"`
	CreatedAt   time.Time `gorm:"column:created_at"`

	ActorUser *User `gorm:"foreignKey:ActorUserID;references:ID"`
}

func (AuditLog) TableName() string { return "audit_logs" }

type ClinicalEvidenceRun struct {
	ID                       string    `gorm:"column:id;primaryKey"`
	RunName                  string    `gorm:"column:run_name"`
	ModelVersion             string    `gorm:"column:model_version"`
	DatasetName              string    `gorm:"column:dataset_name"`
	DatasetVersion           *string   `gorm:"column:dataset_version"`
	SampleCount              int       `gorm:"column:sample_count"`
	PositiveCount            int       `gorm:"column:positive_count"`
	SiteCount                int       `gorm:"column:site_count"`
	Auroc                    float64   `gorm:"column:auroc"`
	Sensitivity              float64   `gorm:"column:sensitivity"`
	Specificity              float64   `gorm:"column:specificity"`
	PPV                      *float64  `gorm:"column:ppv"`
	NPV                      *float64  `gorm:"column:npv"`
	ECE                      *float64  `gorm:"column:ece"`
	Brier                    *float64  `gorm:"column:brier"`
	CalibrationTemperature   *float64  `gorm:"column:calibration_temperature"`
	ThresholdHighSensitivity *float64  `gorm:"column:threshold_high_sensitivity"`
	ThresholdHighSpecificity *float64  `gorm:"column:threshold_high_specificity"`
	RegulatoryStatus         string    `gorm:"column:regulatory_status"`
	StageRecommendation      string    `gorm:"column:stage_recommendation"`
	QAApprovedBy             *string   `gorm:"column:qa_approved_by"`
	MedicalApprovedBy        *string   `gorm:"column:medical_approved_by"`
	ReportPath               *string   `gorm:"column:report_path"`
	Notes                    *string   `gorm:"column:notes"`
	CreatedByUserID          *string   `gorm:"column:created_by_user_id"`
	CreatedAt                time.Time `gorm:"column:created_at"`
	UpdatedAt                time.Time `gorm:"column:updated_at"`

	CreatedBy *User `gorm:"foreignKey:CreatedByUserID;references:ID"`
}

func (ClinicalEvidenceRun) TableName() string { return "clinical_evidence_runs" }

type OpsIncident struct {
	ID             string     `gorm:"column:id;primaryKey"`
	Source         string     `gorm:"column:source"`
	Severity       string     `gorm:"column:severity"`
	Status         string     `gorm:"column:status"`
	Title          string     `gorm:"column:title"`
	Detail         *string    `gorm:"column:detail"`
	OwnerUserID    *string    `gorm:"column:owner_user_id"`
	OpenedAt       time.Time  `gorm:"column:opened_at"`
	AcknowledgedAt *time.Time `gorm:"column:acknowledged_at"`
	ResolvedAt     *time.Time `gorm:"column:resolved_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`

	Owner *User `gorm:"foreignKey:OwnerUserID;references:ID"`
}

func (OpsIncident) TableName() string { return "ops_incidents" }

type PatientConsent struct {
	ID               string    `gorm:"column:id;primaryKey"`
	PatientID        string    `gorm:"column:patient_id"`
	ConsentType      string    `gorm:"column:consent_type"`
	ConsentVersion   string    `gorm:"column:consent_version"`
	Accepted         bool      `gorm:"column:accepted"`
	AcceptedAt       time.Time `gorm:"column:accepted_at"`
	AcceptedByUserID string    `gorm:"column:accepted_by_user_id"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (PatientConsent) TableName() string { return "patient_consents" }

type IdempotencyRecord struct {
	ID             string    `gorm:"column:id;primaryKey"`
	Scope          string    `gorm:"column:scope;uniqueIndex:uidx_idem_scope_key,priority:1"`
	Key            string    `gorm:"column:key;uniqueIndex:uidx_idem_scope_key,priority:2"`
	RequestHash    string    `gorm:"column:request_hash"`
	ResponseStatus int       `gorm:"column:response_status"`
	ResponseBody   []byte    `gorm:"column:response_body"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	ExpiresAt      time.Time `gorm:"column:expires_at;index"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_records" }

type OutboxEvent struct {
	ID             string     `gorm:"column:id;primaryKey"`
	EventType      string     `gorm:"column:event_type;index"`
	AggregateType  string     `gorm:"column:aggregate_type;index"`
	AggregateID    string     `gorm:"column:aggregate_id;index"`
	Payload        []byte     `gorm:"column:payload"`
	IdempotencyKey *string    `gorm:"column:idempotency_key"`
	Status         string     `gorm:"column:status;index"`
	Attempts       int        `gorm:"column:attempts"`
	LastError      *string    `gorm:"column:last_error"`
	PublishedAt    *time.Time `gorm:"column:published_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (OutboxEvent) TableName() string { return "outbox_events" }
