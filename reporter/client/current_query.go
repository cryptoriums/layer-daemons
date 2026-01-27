package client

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/tellor-io/layer/utils"
	oracletypes "github.com/tellor-io/layer/x/oracle/types"
)

func (c *Client) CurrentQuery(ctx context.Context) ([]byte, *oracletypes.QueryMeta, error) {
	response, err := c.OracleQueryClient.CurrentCyclelistQuery(ctx, &oracletypes.QueryCurrentCyclelistQueryRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("error calling 'CurrentCyclelistQuery': %w", err)
	}
	querydata, err := utils.QueryBytesFromString(response.QueryData)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing query id from response: %w", err)
	}

	queryID := hex.EncodeToString(utils.QueryIDFromData(querydata))
	if queryID != c.lastLoggedCycleQueryID {
		c.logger.Info("ReporterDaemon", "current query id in cycle list", queryID)
		c.lastLoggedCycleQueryID = queryID
	}
	return querydata, response.QueryMeta, nil
}
