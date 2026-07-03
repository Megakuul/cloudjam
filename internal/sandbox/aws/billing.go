package aws

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// Bill aggregates the spend of one member account via cost explorer on the
// management (payer) account. Cost explorer data is delayed by up to ~24h,
// so this catches leaks late; Check covers the realtime side with resource
// heuristics.
func (r *Repository) Bill(ctx context.Context, id string) (*sandbox.Bill, error) {
	if _, err := r.readAccount(ctx, id); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	bill := &sandbox.Bill{
		Account: id,
		Window:  r.config.BillingWindow,
		Checked: now,
	}

	input := &costexplorer.GetCostAndUsageInput{
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &cetypes.DateInterval{
			Start: new(now.AddDate(0, 0, -r.config.BillingWindow).Format("2006-01-02")),
			End:   new(now.AddDate(0, 0, 1).Format("2006-01-02")),
		},
		Filter: &cetypes.Expression{
			Dimensions: &cetypes.DimensionValues{
				Key:    cetypes.DimensionLinkedAccount,
				Values: []string{id},
			},
		},
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: new("SERVICE")},
		},
	}

	services := map[string]float64{}
	for {
		page, err := r.costexplorer.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to get cost and usage of account %q: %w", id, err)
		}
		for _, day := range page.ResultsByTime {
			daily := 0.0
			for _, group := range day.Groups {
				metric, ok := group.Metrics["UnblendedCost"]
				if !ok {
					continue
				}
				amount, err := strconv.ParseFloat(*metric.Amount, 64)
				if err != nil {
					continue
				}
				if bill.Currency == "" && metric.Unit != nil {
					bill.Currency = *metric.Unit
				}
				daily += amount
				if len(group.Keys) > 0 {
					services[group.Keys[0]] += amount
				}
			}
			bill.Daily = append(bill.Daily, sandbox.Cost{
				Date:   *day.TimePeriod.Start,
				Amount: daily,
			})
			bill.Total += daily
			if daily > r.config.DailyBudget {
				bill.Leaking = true
			}
		}
		if page.NextPageToken == nil {
			break
		}
		input.NextPageToken = page.NextPageToken
	}

	for service, amount := range services {
		bill.Services = append(bill.Services, sandbox.Cost{
			Service: service,
			Amount:  amount,
		})
	}
	sort.Slice(bill.Services, func(i, j int) bool {
		return bill.Services[i].Amount > bill.Services[j].Amount
	})
	return bill, nil
}

// scanBilling converts the billing state into check findings.
func (r *Repository) scanBilling(ctx context.Context, id string) []sandbox.Finding {
	bill, err := r.Bill(ctx, id)
	if err != nil {
		return []sandbox.Finding{{
			Severity: sandbox.SeverityWarning,
			Resource: "billing",
			Message:  fmt.Sprintf("failed to read billing data: %v", err),
		}}
	}

	peak := sandbox.Cost{}
	for _, day := range bill.Daily {
		if day.Amount > peak.Amount {
			peak = day
		}
	}

	finding := sandbox.Finding{
		Severity: sandbox.SeverityInfo,
		Resource: "billing",
		Message: fmt.Sprintf(
			"spent %.2f %s over the last %d days, peak day %s at %.2f (budget %.2f/day); billing data lags up to ~24h",
			bill.Total, bill.Currency, bill.Window, peak.Date, peak.Amount, r.config.DailyBudget,
		),
	}
	if bill.Leaking {
		finding.Severity = sandbox.SeverityCritical
		finding.Burn = fmt.Sprintf("~$%.2f/day", peak.Amount)
	}
	if len(bill.Services) > 0 {
		finding.Message += fmt.Sprintf("; top service %q at %.2f", bill.Services[0].Service, bill.Services[0].Amount)
	}
	return []sandbox.Finding{finding}
}
