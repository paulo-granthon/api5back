package service

import (
	"context"
	"fmt"
	"time"

	"api5back/ent"
	"api5back/ent/dimprocess"
	"api5back/ent/dimvacancy"
	"api5back/ent/facthiringprocess"
	"api5back/src/processing"

	"github.com/jackc/pgx/v5/pgtype"
)

type MetricsService struct {
	dbClient *ent.Client
}

type MetricsData struct {
	VacancySummary    processing.VacancyStatusSummary      `json:"vacancyStatus"`
	CardInfos         processing.CardInfos                 `json:"cards"`
	AverageHiringTime processing.AverageHiringTimePerMonth `json:"averageHiringTime"`
}

type GetMetricsFilter struct {
	HiringProcessName string `json:"hiringProcess"`
	VacancyName       string `json:"vacancy"`
	StartDate         string `json:"startDate"`
	EndDate           string `json:"endDate"`
}

func NewMetricsService(dbclient *ent.Client) *MetricsService {
	return &MetricsService{dbClient: dbclient}
}

func (s *MetricsService) GetMetrics(
	ctx context.Context,
	filter GetMetricsFilter,
) (*MetricsData, error) {
	query := s.dbClient.
		FactHiringProcess.
		Query().
		WithDimVacancy().
		WithDimProcess().
		WithHiringProcessCandidates()

	if filter.HiringProcessName != "" {
		query = query.Where(
			facthiringprocess.HasDimProcessWith(
				dimprocess.TitleContains(
					filter.HiringProcessName,
				),
			),
		)
	}

	if filter.VacancyName != "" {
		query = query.Where(
			facthiringprocess.HasDimVacancyWith(
				dimvacancy.TitleContains(
					filter.VacancyName,
				),
			),
		)
	}

	if filter.StartDate != "" {
		hiringProcessStartDate, err := ParseStringToPgtypeDate(
			"2006-01-02",
			filter.StartDate,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse `StartDate`: %w",
				err,
			)
		}

		query = query.Where(
			facthiringprocess.HasDimVacancyWith(
				dimvacancy.ClosingDateGTE(
					&hiringProcessStartDate,
				),
			),
		)
	}

	if filter.EndDate != "" {
		hiringProcessEndDate, err := ParseStringToPgtypeDate(
			"2006-01-02",
			filter.EndDate,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"could not parse `EndDate`: %w",
				err,
			)
		}

		query = query.Where(
			facthiringprocess.HasDimVacancyWith(
				dimvacancy.ClosingDateLTE(
					&hiringProcessEndDate,
				),
			),
		)
	}

	hiringProcess, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"could not retrieve `FactHiringProcess` data: %w",
			err,
		)
	}

	var errors []error

	cardInfo, err := processing.ComputingCardInfo(hiringProcess)
	if err != nil {
		errors = append(errors, fmt.Errorf(
			"could not calculate `CardInfo` data: %w",
			err,
		))
	}

	vacancyInfo, err := processing.GenerateVacancyStatusSummary(hiringProcess)
	if err != nil {
		errors = append(errors, fmt.Errorf(
			"could not generate `VacancyStatus` summary: %w",
			err,
		))
	}

	averageHiringTime, err := processing.GenerateAverageHiringTime(hiringProcess)
	if err != nil {
		errors = append(errors, fmt.Errorf(
			"could not generate `AvgHiringTime` data: %w",
			err,
		))
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf(
			"failed to get metrics: %v",
			errors,
		)
	}

	return &MetricsData{
		CardInfos:         cardInfo,
		VacancySummary:    vacancyInfo,
		AverageHiringTime: averageHiringTime,
	}, nil
}

func ParseStringToPgtypeDate(
	layout string,
	dateString string,
) (pgDate pgtype.Date, err error) {
	t, err := time.Parse(layout, dateString)
	if err != nil {
		return pgDate, fmt.Errorf("failed to parse date: %v", err)
	}

	return pgtype.Date{
		Time:  t,
		Valid: true,
	}, nil
}
