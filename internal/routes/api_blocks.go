package routes

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// Blocks handlers live here (rather than api_v1.go) so the block
// surface stays in one file. Same conventions as the goals
// handlers: thin bind → validate → controller → DTO, sentinel
// errors become 400s, ErrBlockNotFound becomes a 404.

// SetBlocksController attaches the Blocks orchestrator. Kept as a
// setter (rather than NewHandler params) so existing construction
// sites and tests are untouched — same pattern as SetCoachService.
func (h *Handler) SetBlocksController(ctrl *controllers.BlocksController) {
	h.blocksCtrl = ctrl
}

// blockValidationError maps block controller sentinels to a client
// message. The second return is false for non-validation errors
// (caller responds 500).
func blockValidationError(err error) (string, bool) {
	switch err {
	case controllers.ErrBlockNameRequired:
		return "block name is required", true
	case controllers.ErrBlockNameTooLong:
		return "block name must be 100 characters or less", true
	case controllers.ErrBlockDescriptionLong:
		return "block description must be 1000 characters or less", true
	case controllers.ErrBlockTypeInvalid:
		return "block type must be standard, circuit, amrap or emom", true
	case controllers.ErrBlockRoundsRequired:
		return "rounds must be at least 1 for circuits and EMOMs", true
	case controllers.ErrBlockRoundsTooMany:
		return "rounds must be 100 or less", true
	case controllers.ErrBlockRoundsUnused:
		return "rounds only applies to circuits and EMOMs", true
	case controllers.ErrBlockRestInvalid:
		return "rest must be between 0 and 3600 seconds", true
	case controllers.ErrBlockRestUnused:
		return "rest only applies to circuits", true
	case controllers.ErrBlockTimeCapRequired:
		return "time cap is required for AMRAPs", true
	case controllers.ErrBlockTimeCapInvalid:
		return "time cap must be between 60 and 86400 seconds", true
	case controllers.ErrBlockTimeCapUnused:
		return "time cap only applies to AMRAPs", true
	case controllers.ErrBlockIntervalRequired:
		return "interval is required for EMOMs", true
	case controllers.ErrBlockIntervalInvalid:
		return "interval must be between 15 and 3600 seconds", true
	case controllers.ErrBlockIntervalUnused:
		return "interval only applies to EMOMs", true
	case controllers.ErrBlockItemsRequired:
		return "a block needs at least 1 exercise", true
	case controllers.ErrBlockItemsTooMany:
		return "a block can hold at most 20 exercises", true
	case controllers.ErrBlockExerciseRequired:
		return "block item exercise is required", true
	case controllers.ErrBlockExerciseNotFound:
		return "exercise not found", true
	case controllers.ErrBlockTargetTooLong:
		return "target must be 500 characters or less", true
	}
	return "", false
}

// blockItemsToInputs converts request item DTOs into controller
// inputs. Position is implicit (slice order).
func blockItemsToInputs(ins []CreateBlockItemRequest) []controllers.BlockItemInput {
	out := make([]controllers.BlockItemInput, 0, len(ins))
	for _, in := range ins {
		out = append(out, controllers.BlockItemInput{
			ExerciseID: in.ExerciseID,
			TargetText: in.TargetText,
		})
	}
	return out
}

// APIListBlocks handles GET /api/v1/blocks. Returns the
// authenticated user's block summaries (newest first).
func (h *Handler) APIListBlocks(c echo.Context) error {
	claims := GetClaims(c)
	blocks, err := h.blocksCtrl.ListBlocks(claims.UserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load blocks"})
	}
	return c.JSON(http.StatusOK, map[string]any{"blocks": BlockSummariesFromModels(blocks)})
}

// APICreateBlock handles POST /api/v1/blocks. Validates the body,
// resolves each item's exercise, and returns the created block
// with items.
func (h *Handler) APICreateBlock(c echo.Context) error {
	var in CreateBlockRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	created, err := h.blocksCtrl.CreateBlock(claims.UserID, controllers.CreateBlockInput{
		Name:            in.Name,
		Description:     in.Description,
		Type:            models.BlockType(in.Type),
		Rounds:          in.Rounds,
		RestSeconds:     in.RestSeconds,
		TimeCapSeconds:  in.TimeCapSeconds,
		IntervalSeconds: in.IntervalSeconds,
		Items:           blockItemsToInputs(in.Items),
	})
	if err != nil {
		if msg, ok := blockValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to create block"})
	}
	return c.JSON(http.StatusCreated, BlockFromModel(*created))
}

// APIGetBlock handles GET /api/v1/blocks/:id. Returns 404 when the
// block is missing or owned by another user.
func (h *Handler) APIGetBlock(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	b, err := h.blocksCtrl.GetBlock(id, claims.UserID)
	if err != nil {
		if errors.Is(err, controllers.ErrBlockNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "block not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to load block"})
	}
	return c.JSON(http.StatusOK, BlockFromModel(*b))
}

// APIUpdateBlock handles PUT /api/v1/blocks/:id. Items are fully
// replaced (same shape as create).
func (h *Handler) APIUpdateBlock(c echo.Context) error {
	var in UpdateBlockRequest
	if err := c.Bind(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: "invalid request body"})
	}
	if err := h.validator.ValidateStruct(&in); err != nil {
		return c.JSON(http.StatusBadRequest, APIError{Error: friendlyValidationError(err)})
	}

	claims := GetClaims(c)
	id := c.Param("id")
	updated, err := h.blocksCtrl.UpdateBlock(id, claims.UserID, controllers.UpdateBlockInput{
		Name:            in.Name,
		Description:     in.Description,
		Type:            models.BlockType(in.Type),
		Rounds:          in.Rounds,
		RestSeconds:     in.RestSeconds,
		TimeCapSeconds:  in.TimeCapSeconds,
		IntervalSeconds: in.IntervalSeconds,
		Items:           blockItemsToInputs(in.Items),
	})
	if err != nil {
		if errors.Is(err, controllers.ErrBlockNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "block not found"})
		}
		if msg, ok := blockValidationError(err); ok {
			return c.JSON(http.StatusBadRequest, APIError{Error: msg})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to update block"})
	}
	return c.JSON(http.StatusOK, BlockFromModel(*updated))
}

// APIDeleteBlock handles DELETE /api/v1/blocks/:id. Hard delete
// scoped to the authenticated user. Returns 204.
func (h *Handler) APIDeleteBlock(c echo.Context) error {
	claims := GetClaims(c)
	id := c.Param("id")

	if err := h.blocksCtrl.DeleteBlock(id, claims.UserID); err != nil {
		if errors.Is(err, controllers.ErrBlockNotFound) {
			return c.JSON(http.StatusNotFound, APIError{Error: "block not found"})
		}
		return c.JSON(http.StatusInternalServerError, APIError{Error: "failed to delete block"})
	}
	return c.NoContent(http.StatusNoContent)
}
