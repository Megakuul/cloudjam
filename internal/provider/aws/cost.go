package aws

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"codeberg.org/megakuul/cloudjam/internal/provider"
)

func (r *Provider) Cost(ctx context.Context, id string, window time.Duration) ([]provider.Cost, error) {
	now := time.Now().UTC()
	input := &costexplorer.GetCostAndUsageInput{
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &cetypes.DateInterval{
			Start: new(now.Add(-window).Format(time.DateOnly)),
			End:   new(now.AddDate(0, 0, 1).Format(time.DateOnly)),
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

	costs := []provider.Cost{}
	for {
		page, err := r.costexplorer.GetCostAndUsage(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to get cost and usage of account %q: %w", id, err)
		}
		for _, day := range page.ResultsByTime {
			if day.TimePeriod == nil || day.TimePeriod.Start == nil {
				continue
			}
			date, err := time.Parse(time.DateOnly, *day.TimePeriod.Start)
			if err != nil {
				return nil, fmt.Errorf("failed to parse cost date %q: %w", *day.TimePeriod.Start, err)
			}
			for _, group := range day.Groups {
				metric, ok := group.Metrics["UnblendedCost"]
				if !ok || metric.Amount == nil || len(group.Keys) < 1 {
					continue
				}
				// cost explorer reports in the currency of the payer account, we only speak usd.
				if metric.Unit != nil && *metric.Unit != "USD" {
					return nil, fmt.Errorf("cost explorer reported %q, only usd is supported", *metric.Unit)
				}
				amount, err := strconv.ParseFloat(*metric.Amount, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse cost amount %q: %w", *metric.Amount, err)
				}
				costs = append(costs, provider.Cost{
					Service: group.Keys[0],
					Date:    date,
					Amount:  amount,
				})
			}
		}
		if page.NextPageToken == nil {
			return costs, nil
		}
		input.NextPageToken = page.NextPageToken
	}
}
