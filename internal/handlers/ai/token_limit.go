package ai

import (
	"context"
	"fmt"

	"fitness-backend/internal/db"
	"github.com/gofiber/fiber/v2"
)

// dailyTokenLimit is the maximum number of tokens a user may consume per day
// across all AI endpoints. At 800–5000 tokens per request this allows roughly
// 4–25 interactions before hitting the cap.
const dailyTokenLimit = 20000

// getDailyTokensUsed returns how many tokens the user has consumed today.
// Returns 0 on any error so a DB failure does not block the user.
func getDailyTokensUsed(userID string) (int, error) {
	var used int
	err := db.DB.QueryRow(
		context.Background(),
		`SELECT tokens_used FROM daily_token_usage WHERE user_id = $1 AND usage_date = CURRENT_DATE`,
		userID,
	).Scan(&used)
	if err != nil {
		// No row = 0 tokens used today; surface real errors as warnings only.
		return 0, nil
	}
	return used, nil
}

// GetTokenUsage returns the user's token consumption and limit for today.
func GetTokenUsage(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	used, _ := getDailyTokensUsed(userID)
	return c.JSON(fiber.Map{
		"tokens_used": used,
		"daily_limit": dailyTokenLimit,
	})
}

// addDailyTokens increments the user's daily token counter by the given amount,
// creating the row if it doesn't exist yet.
func addDailyTokens(userID string, tokens int) {
	_, err := db.DB.Exec(
		context.Background(),
		`INSERT INTO daily_token_usage (user_id, usage_date, tokens_used)
		 VALUES ($1, CURRENT_DATE, $2)
		 ON CONFLICT (user_id, usage_date) DO UPDATE
		 SET tokens_used = daily_token_usage.tokens_used + $2`,
		userID, tokens,
	)
	if err != nil {
		fmt.Printf("Warning: failed to record token usage for user %s: %v\n", userID, err)
	}
}
