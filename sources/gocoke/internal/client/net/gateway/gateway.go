package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gocoke/internal/client/net/amf"
)

// postAMF posts an AMF0 remoting envelope and returns the decoded first body.
func postAMF(httpClient *http.Client, url string, target string, params []any) (any, error) {
	reqBody, err := amf.EncodeRemotingRequest(target, params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amf")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("gateway: http %d: %s", resp.StatusCode, string(b))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	val, err := amf.DecodeRemotingResponse(b)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// postAMFOne is a helper that unwraps the return, and if it's a RemotingError, converts to Go error.
func postAMFOne(httpClient *http.Client, url string, target string, params []any) (any, error) {
	v, err := postAMF(httpClient, url, target, params)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		if m["__amf_error_type"] != nil {
			return nil, fmt.Errorf("gateway error %v: %v", m["__amf_error_type"], m["__amf_error_message"])
		}
	}
	return v, nil
}

// GetTip calls FlashDenizenManager.getTip and expects a string response.
func GetTip(httpClient *http.Client, url string) (string, error) {
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.denizen.FlashDenizenManager.getTip", nil)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New("unexpected response type for getTip")
	}
	return s, nil
}

// GetDenizenBalance calls FlashDenizenManager.getDenizenBalance(screenName, secret)
// and returns the integer balance.
func GetDenizenBalance(httpClient *http.Client, url string, screenName string, secret string) (int, error) {
	params := []any{screenName, secret}
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.denizen.FlashDenizenManager.getDenizenBalance", params)
	if err != nil {
		return 0, err
	}
	switch x := v.(type) {
	case float64:
		return int(x), nil
	case int:
		return x, nil
	default:
		return 0, errors.New("unexpected response type for getDenizenBalance")
	}
}

// GetBackpack calls FlashPossessionManager.getBackpack(screenName, secret) and
// returns the raw decoded AMF value (typically a map[string]any with typed alias).
func GetBackpack(httpClient *http.Client, url string, screenName string, secret string) (any, error) {
	params := []any{screenName, secret}
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.catalog.FlashPossessionManager.getBackpack", params)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetPossessionList calls FlashPossessionManager.getPossessionList(screenName, secret, idsArray)
// and returns the raw decoded AMF value (array of possessions or similar typed objects).
func GetPossessionList(httpClient *http.Client, url string, screenName string, secret string, ids []int) (any, error) {
	idsAny := make([]any, 0, len(ids))
	for _, id := range ids {
		idsAny = append(idsAny, id)
	}
	params := []any{screenName, secret, idsAny}
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.catalog.FlashPossessionManager.getPossessionList", params)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetPossessionsInBackpack mimics the Lingo flow: getBackpack then getPossessionList if needed
func GetPossessionsInBackpack(httpClient *http.Client, url string, screenName string, secret string) ([]map[string]any, error) {
	// Call getBackpack per Lingo's getPossessionsInBackpack flow
	fmt.Printf("Gateway: GetPossessionsInBackpack called for user=%s\n", screenName)
	backpackV, err := GetBackpack(httpClient, url, screenName, secret)
	if err != nil {
		fmt.Printf("Gateway: GetBackpack failed: %v\n", err)
		return nil, err
	}

	fmt.Printf("Gateway: GetBackpack response type: %T\n", backpackV)

	// Parse the backpack response (all fields are lowercase per ActionScript)
	var result []map[string]any
	if backpackMap, ok := backpackV.(map[string]any); ok {
		// Log all keys to see what's available
		fmt.Printf("Gateway: Backpack keys: ")
		for k := range backpackMap {
			fmt.Printf("%s, ", k)
		}
		fmt.Println()

		// Check if possessionlist is already populated with full Possession objects (lowercase!)
		if possessionList, ok := backpackMap["possessionlist"]; ok && possessionList != nil {
			fmt.Printf("Gateway: possessionlist exists, type: %T\n", possessionList)
			if possArray, ok := possessionList.([]any); ok {
				fmt.Printf("Gateway: Found possessionlist array with %d items\n", len(possArray))
				// DEBUG: Log full possessionlist for inspection (limit size to avoid flooding)
				for i, poss := range possArray {
					fmt.Printf("Gateway: possession[%d] = %v\n", i, poss)
				}
				if len(possArray) > 0 {
					// possessionList already has full objects, use them
					for i, poss := range possArray {
						if possMap, ok := poss.(map[string]any); ok {
							// Log first item's keys to understand structure
							if i == 0 {
								fmt.Printf("Gateway: First possession keys: ")
								for k := range possMap {
									fmt.Printf("%s, ", k)
								}
								fmt.Println()
							}

							item := make(map[string]any)

							// Get catalogItem (prodID) - try both cases
							if catItem, ok := possMap["catalogItem"]; ok {
								switch v := catItem.(type) {
								case float64:
									item["prodID"] = int(v)
								case int:
									item["prodID"] = v
								default:
									item["prodID"] = catItem
								}
							} else if catItem, ok := possMap["catalogitem"]; ok {
								switch v := catItem.(type) {
								case float64:
									item["prodID"] = int(v)
								case int:
									item["prodID"] = v
								default:
									item["prodID"] = catItem
								}
							}

							// Get id (posID) - ActionScript field is id
							if id, ok := possMap["id"]; ok {
								switch v := id.(type) {
								case float64:
									item["posID"] = int(v)
								case int:
									item["posID"] = v
								default:
									item["posID"] = id
								}
							}

							// Log if we're missing essential fields
							if item["prodID"] == nil || item["posID"] == nil {
								if i < 3 { // Log first few items for debugging
									fmt.Printf("Gateway: Item %d missing fields - prodID: %v, posID: %v\n", i, item["prodID"], item["posID"])
								}
								// Still add the item even if incomplete for debugging
							}

							// Get properties/attributes
							var attrs map[string]any
							if props, ok := possMap["properties"]; ok && props != nil {
								if propsMap, ok := props.(map[string]any); ok {
									attrs = propsMap
								}
							}
							if attrs == nil {
								attrs = make(map[string]any)
							}
							// Fallback: parse ownerName/songName out of propertiesstring
							if _, ok := attrs["ownerName"]; !ok {
								if owner := extractPropString(attrs, "ownerName", possMap); owner != "" {
									attrs["ownerName"] = owner
								}
							}
							if _, ok := attrs["songName"]; !ok {
								if song := extractPropString(attrs, "songName", possMap); song != "" {
									attrs["songName"] = song
								}
							}
							item["attributes"] = attrs

							// If this is a burned CD (prodID 89), derive display name from ownerName/songName
							if pid, ok := item["prodID"].(int); ok && pid == 89 {
								ownerName := extractPropString(attrs, "ownerName", possMap)
								songName := extractPropString(attrs, "songName", possMap)
								if songName != "" {
									if ownerName != "" {
										item["name"] = fmt.Sprintf("%s: %s", ownerName, songName)
									} else {
										item["name"] = songName
									}
								}
							}

							result = append(result, item)
						}
					}
					fmt.Printf("Gateway: Returning %d items from possessionList\n", len(result))
					return result, nil
				}
			}
		}

		// possessionList not populated, use possessionids and fetch details
		if ids, ok := backpackMap["possessionids"]; ok {
			fmt.Printf("Gateway: Found possessionids field, type: %T\n", ids)
			var possessionIDs []int
			switch v := ids.(type) {
			case []any:
				for _, id := range v {
					if idNum, ok := id.(float64); ok {
						possessionIDs = append(possessionIDs, int(idNum))
					} else if idNum, ok := id.(int); ok {
						possessionIDs = append(possessionIDs, idNum)
					}
				}
			}

			fmt.Printf("Gateway: Found %d possession IDs in possessionids array\n", len(possessionIDs))
			if len(possessionIDs) > 0 {
				// Fetch the actual possession details
				possessionsV, err := GetPossessionList(httpClient, url, screenName, secret, possessionIDs)
				if err != nil {
					return nil, err
				}

				// Parse possessions into structured format
				if possArray, ok := possessionsV.([]any); ok {
					fmt.Printf("Gateway: Processing %d items from getPossessionList\n", len(possArray))
					for i, poss := range possArray {
						fmt.Printf("Gateway: getPossessionList[%d] = %v\n", i, poss)
						if possMap, ok := poss.(map[string]any); ok {
							// Log first item's keys to understand structure
							if i == 0 {
								fmt.Printf("Gateway: First possession from getPossessionList keys: ")
								for k := range possMap {
									fmt.Printf("%s, ", k)
								}
								fmt.Println()
							}

							// Extract key fields per ActionScript Possession
							item := make(map[string]any)

							// Get catalogItem (prodID)
							if catItem, ok := possMap["catalogItem"]; ok {
								switch v := catItem.(type) {
								case float64:
									item["prodID"] = int(v)
								case int:
									item["prodID"] = v
								}
							}

							// Get id (posID)
							if id, ok := possMap["id"]; ok {
								switch v := id.(type) {
								case float64:
									item["posID"] = int(v)
								case int:
									item["posID"] = v
								}
							}

							// Skip items that are placed in studios (not in backpack)
							if placedIn, ok := possMap["placedIn"]; ok && placedIn != nil && placedIn != "" && placedIn != "Backpack" {
								fmt.Printf("Gateway: Skipping item %v placed in %v\n", item["posID"], placedIn)
								continue
							}

							// Get properties/attributes
							var attrs map[string]any
							if props, ok := possMap["properties"]; ok && props != nil {
								if propsMap, ok := props.(map[string]any); ok {
									attrs = propsMap
								}
							}
							if attrs == nil {
								attrs = make(map[string]any)
							}
							item["attributes"] = attrs

							// If this is a burned CD (prodID 89), derive display name from ownerName/songName
							if pid, ok := item["prodID"].(int); ok && pid == 89 {
								ownerName := extractPropString(attrs, "ownerName", possMap)
								songName := extractPropString(attrs, "songName", possMap)
								if songName != "" {
									if ownerName != "" {
										item["name"] = fmt.Sprintf("%s: %s", ownerName, songName)
									} else {
										item["name"] = songName
									}
								}
							}

							result = append(result, item)
						}
					}
				}
			}
		}
	} else {
		fmt.Printf("Gateway: Backpack response is not a map, type: %T\n", backpackV)
	}

	fmt.Printf("Gateway: Returning %d items from backpack\n", len(result))
	return result, nil
}

// GetAllPrivateStudios calls FlashStudioManager.getAllPrivateStudios() and returns the raw value.
func GetAllPrivateStudios(httpClient *http.Client, url string) (any, error) {
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.getAllPrivateStudios", nil)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetAllPublicStudios calls FlashStudioManager.getAllPublicStudios() and returns the raw value.
func GetAllPublicStudios(httpClient *http.Client, url string) (any, error) {
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.getAllPublicStudios", nil)
	if err != nil {
		return nil, err
	}
	return v, nil
}


// GetProductList calls FlashCatalogManager.getProductList() and returns a map of prodID -> price.
func GetProductList(httpClient *http.Client, url string) (map[int]int, error) {
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.catalog.FlashCatalogManager.getProductList", nil)
	if err != nil {
		return nil, err
	}
	prices := make(map[int]int)
	switch arr := v.(type) {
	case []any:
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			// Accept either "id" or "prodId"
			id := 0
			if val, ok := m["id"]; ok {
				switch x := val.(type) {
				case float64:
					id = int(x)
				case int:
					id = x
				}
			} else if val, ok := m["prodId"]; ok {
				switch x := val.(type) {
				case float64:
					id = int(x)
				case int:
					id = x
				}
			}
			if id == 0 {
				continue
			}
			price := 0
			if val, ok := m["price"]; ok {
				switch x := val.(type) {
				case float64:
					price = int(x)
				case int:
					price = x
				}
			}
			if price > 0 {
				prices[id] = price
			}
		}
	}
	return prices, nil
}

// PrePurchaseItem calls FlashPossessionManager.prePurchaseItem(screenName, secret, catalogItemId, attributes)
// and returns balance, price, catalogItemId, and properties string.
type PrePurchaseResult struct {
	Balance        int
	Price          int
	CatalogItemID  int
	Properties     string
}

func PrePurchaseItem(httpClient *http.Client, url, screenName, secret string, catalogItemID int, attributes string) (*PrePurchaseResult, error) {
	params := []any{screenName, secret, catalogItemID, attributes}
	v, err := postAMFOne(httpClient, url, "com.studiocom.sanfrancisco.catalog.FlashPossessionManager.prePurchaseItem", params)
	if err != nil {
		return nil, err
	}
	res := &PrePurchaseResult{}
	if m, ok := v.(map[string]any); ok {
		// balance
		if val, ok := m["balance"]; ok {
			switch x := val.(type) {
			case float64:
				res.Balance = int(x)
			case int:
				res.Balance = x
			}
		}
		// price
		if val, ok := m["price"]; ok {
			switch x := val.(type) {
			case float64:
				res.Price = int(x)
			case int:
				res.Price = x
			}
		}
		// catalogItemId (try both cases)
		if val, ok := m["catalogItemId"]; ok {
			switch x := val.(type) {
			case float64:
				res.CatalogItemID = int(x)
			case int:
				res.CatalogItemID = x
			}
		} else if val, ok := m["catalogitemid"]; ok {
			switch x := val.(type) {
			case float64:
				res.CatalogItemID = int(x)
			case int:
				res.CatalogItemID = x
			}
		}
		// properties
		if val, ok := m["properties"]; ok {
			if s, ok2 := val.(string); ok2 { res.Properties = s }
		}
	}
	return res, nil
}

// PurchaseItem calls FlashPossessionManager.purchaseItem(screenName, secret, catalogItemId, attributes)
// and returns the raw decoded value (array of possessions or status object).
func PurchaseItem(httpClient *http.Client, url, screenName, secret string, catalogItemID int, attributes string) (any, error) {
	params := []any{screenName, secret, catalogItemID, attributes}
	v, err := postAMFOne(httpClient, url, "com.studiocom.sanfrancisco.catalog.FlashPossessionManager.purchaseItem", params)
	if err != nil { return nil, err }
	return v, nil
}

// GetGameServerByStudioID calls FlashStudioManager.getGameServerByStudioID(studioId)
// and returns the raw decoded value (typed object with fields like server/port/dns etc.).
func GetGameServerByStudioID(httpClient *http.Client, url string, studioID string) (any, error) {
	// Lingo passes (sStudioId, sDNSAndPort). Provide empty string for sDNSAndPort when not connected.
	params := []any{studioID, ""}
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.getGameServerByStudioID", params)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetStudiosByOwnerName calls FlashStudioManager.getByOwnerName(screenName)
// and returns the raw decoded AMF value (typically an array of typed Studio objects).
func GetStudiosByOwnerName(httpClient *http.Client, url string, screenName string) (any, error) {
	params := []any{screenName}
	v, err := postAMF(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.getByOwnerName", params)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// extractPropString attempts to extract a string field (like ownerName or songName)
// from attributes map or by parsing the propertiesstring content if present in the possession map.
func extractPropString(attrs map[string]any, key string, possMap map[string]any) string {
	// 1) Try attributes map directly
	if attrs != nil {
		if v, ok := attrs[key]; ok {
			if s, ok2 := v.(string); ok2 {
				return strings.TrimSpace(s)
			}
		}
	}
	// 2) Try parsing propertiesstring: it looks like [ #ownerName: "Adam", #songName: "..." ]
	if ps, ok := possMap["propertiesstring"]; ok {
		if s, ok2 := ps.(string); ok2 {
			// Look for #key: "value"
			needle := "#" + key + ": \""
			if idx := strings.Index(s, needle); idx >= 0 {
				rest := s[idx+len(needle):]
				if j := strings.Index(rest, "\""); j >= 0 {
					return strings.TrimSpace(rest[:j])
				}
			}
		}
	}
	return ""
}


// CreateStudio calls FlashStudioManager.createStudio via AMF remoting.
func CreateStudio(httpClient *http.Client, url, screenName, secret, studioName, desc string, layout, location int) (map[string]any, error) {
	params := []any{screenName, secret, studioName, desc, float64(layout), float64(location)}
	v, err := postAMFOne(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.createStudio", params)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return nil, errors.New("unexpected response type for createStudio")
}

// ModifyStudio calls FlashStudioManager.modifyStudio via AMF remoting.
func ModifyStudio(httpClient *http.Client, url, screenName, secret, studioID, studioName, desc string) (map[string]any, error) {
	params := []any{screenName, secret, studioID, studioName, desc}
	v, err := postAMFOne(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.modifyStudio", params)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return nil, errors.New("unexpected response type for modifyStudio")
}

// DeleteStudio calls FlashStudioManager.deleteStudio via AMF remoting.
func DeleteStudio(httpClient *http.Client, url, screenName, secret, studioID string) error {
	params := []any{screenName, secret, studioID}
	_, err := postAMFOne(httpClient, url, "com.studiocom.sanfrancisco.studio.FlashStudioManager.deleteStudio", params)
	return err
}
