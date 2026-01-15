package db

import (
	"context"
	"fmt"
	"testing"
)

func TestExampleUsage(t *testing.T) {
	ctx := context.Background()
	database := New()

	database.CreateTable(ctx, TableSchema{
		Name: "users",
		Fields: []Field{
			{Name: "user_id", Type: FieldTypeString},
			{Name: "user_name", Type: FieldTypeString},
			{Name: "f_name", Type: FieldTypeString},
			{Name: "last_name", Type: FieldTypeString},
			{Name: "age", Type: FieldTypeInt},
			{Name: "email", Type: FieldTypeString},
		},
		PrimaryKey: "user_id",
		Indexes:    []string{"email", "user_name"},
	})

	database.CreateTable(ctx, TableSchema{
		Name: "profile_images",
		Fields: []Field{
			{Name: "image_id", Type: FieldTypeString},
			{Name: "user_id", Type: FieldTypeString},
			{Name: "url", Type: FieldTypeString},
			{Name: "is_primary", Type: FieldTypeBool},
		},
		PrimaryKey: "image_id",
		Indexes:    []string{"user_id"},
	})

	database.Insert(ctx, "users", Record{
		"user_id":   "u1",
		"user_name": "johndoe",
		"f_name":    "John",
		"last_name": "Doe",
		"age":       30,
		"email":     "john@example.com",
	})

	database.Insert(ctx, "users", Record{
		"user_id":   "u2",
		"user_name": "janedoe",
		"f_name":    "Jane",
		"last_name": "Doe",
		"age":       28,
		"email":     "jane@example.com",
	})

	database.Insert(ctx, "users", Record{
		"user_id":   "u3",
		"user_name": "bobsmith",
		"f_name":    "Bob",
		"last_name": "Smith",
		"age":       35,
		"email":     "bob@example.com",
	})

	database.Insert(ctx, "profile_images", Record{
		"image_id":   "img1",
		"user_id":    "u1",
		"url":        "https://cdn.example.com/john-avatar.jpg",
		"is_primary": true,
	})

	database.Insert(ctx, "profile_images", Record{
		"image_id":   "img2",
		"user_id":    "u1",
		"url":        "https://cdn.example.com/john-cover.jpg",
		"is_primary": false,
	})

	database.Insert(ctx, "profile_images", Record{
		"image_id":   "img3",
		"user_id":    "u2",
		"url":        "https://cdn.example.com/jane-avatar.jpg",
		"is_primary": true,
	})

	fmt.Println("=== GET single user by primary key ===")
	user, _ := database.Get(ctx, "users", "u1")
	fmt.Printf("User: %s %s (%s)\n", user["f_name"], user["last_name"], user["email"])

	fmt.Println("\n=== SELECT with WHERE filter ===")
	does, _ := database.Select(ctx, "users").
		Where("last_name", "Doe").
		Execute(ctx)
	fmt.Printf("Users with last_name='Doe': %d\n", len(does))
	for _, u := range does {
		fmt.Printf("  - %s %s\n", u["f_name"], u["last_name"])
	}

	fmt.Println("\n=== SELECT with LIMIT ===")
	limited, _ := database.Select(ctx, "users").
		Limit(2).
		Execute(ctx)
	fmt.Printf("First 2 users: %d\n", len(limited))

	fmt.Println("\n=== INNER JOIN users with profile_images ===")
	joined, _ := database.Join("users", "profile_images").
		On("user_id", "user_id").
		Execute(ctx)
	fmt.Printf("Users with images (inner join): %d records\n", len(joined))
	for _, r := range joined {
		fmt.Printf("  - %s: %s\n", r["users.user_name"], r["profile_images.url"])
	}

	fmt.Println("\n=== LEFT JOIN users with profile_images ===")
	leftJoined, _ := database.Join("users", "profile_images").
		On("user_id", "user_id").
		Type(LeftJoin).
		Execute(ctx)
	fmt.Printf("All users with optional images (left join): %d records\n", len(leftJoined))
	for _, r := range leftJoined {
		url := r["profile_images.url"]
		if url == nil {
			url = "(no image)"
		}
		fmt.Printf("  - %s: %v\n", r["users.user_name"], url)
	}

	fmt.Println("\n=== JOIN with WHERE filter ===")
	primaryImages, _ := database.Join("users", "profile_images").
		On("user_id", "user_id").
		Where("profile_images.is_primary", true).
		Execute(ctx)
	fmt.Printf("Users with primary images: %d records\n", len(primaryImages))
	for _, r := range primaryImages {
		fmt.Printf("  - %s: %s\n", r["users.user_name"], r["profile_images.url"])
	}

	fmt.Println("\n=== UPSERT (update existing) ===")
	database.Upsert(ctx, "users", Record{
		"user_id": "u1",
		"age":     31,
	})
	updated, _ := database.Get(ctx, "users", "u1")
	fmt.Printf("Updated age for John: %v\n", updated["age"])

	fmt.Println("\n=== COUNT ===")
	count, _ := database.Count(ctx, "users")
	fmt.Printf("Total users: %d\n", count)

	fmt.Println("\n=== DELETE ===")
	database.Delete(ctx, "users", "u3")
	countAfter, _ := database.Count(ctx, "users")
	fmt.Printf("Users after delete: %d\n", countAfter)

	fmt.Println("\n=== INSERT BATCH ===")
	batchUsers := []Record{
		{"user_id": "u10", "user_name": "alice", "f_name": "Alice", "last_name": "Wonder", "age": 29, "email": "alice@example.com"},
		{"user_id": "u11", "user_name": "charlie", "f_name": "Charlie", "last_name": "Brown", "age": 32, "email": "charlie@example.com"},
		{"user_id": "u12", "user_name": "diana", "f_name": "Diana", "last_name": "Prince", "age": 27, "email": "diana@example.com"},
	}
	err := database.InsertBatch(ctx, "users", batchUsers)
	if err != nil {
		fmt.Printf("InsertBatch failed: %v\n", err)
	} else {
		countAfterBatch, _ := database.Count(ctx, "users")
		fmt.Printf("Users after batch insert: %d\n", countAfterBatch)
	}

	fmt.Println("\n=== UPSERT BATCH ===")
	upsertRecords := []Record{
		{"user_id": "u10", "age": 30},                          // Update existing - only update age
		{"user_id": "u11", "email": "charlie_new@example.com"}, // Update existing - only update email
		{"user_id": "u20", "user_name": "newuser", "f_name": "New", "last_name": "User", "age": 25, "email": "new@example.com"}, // Insert new
	}
	err = database.UpsertBatch(ctx, "users", upsertRecords)
	if err != nil {
		fmt.Printf("UpsertBatch failed: %v\n", err)
	} else {
		countAfterUpsert, _ := database.Count(ctx, "users")
		fmt.Printf("Users after batch upsert: %d\n", countAfterUpsert)
		// Verify partial update preserved other fields
		alice, _ := database.Get(ctx, "users", "u10")
		fmt.Printf("Alice's updated age: %v, preserved name: %s\n", alice["age"], alice["f_name"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT (DO NOTHING) ===")
	// Try to insert a duplicate - should be silently ignored
	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id":   "u1", // Already exists
		"user_name": "duplicate",
		"f_name":    "Duplicate",
		"last_name": "User",
		"age":       99,
		"email":     "dup@example.com",
	}, ConflictOptions{Action: ConflictDoNothing})
	if err != nil {
		fmt.Printf("InsertOnConflict DO NOTHING failed: %v\n", err)
	} else {
		// Verify the original record was not modified
		original, _ := database.Get(ctx, "users", "u1")
		fmt.Printf("Original user preserved (age should be 31): %v\n", original["age"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT (DO UPDATE - all fields) ===")
	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id":   "u2", // Already exists (Jane)
		"user_name": "jane_updated",
		"f_name":    "Jane",
		"last_name": "Smith", // Changed from Doe
		"age":       29,      // Changed from 28
		"email":     "jane_new@example.com",
	}, ConflictOptions{Action: ConflictDoUpdate})
	if err != nil {
		fmt.Printf("InsertOnConflict DO UPDATE failed: %v\n", err)
	} else {
		updated, _ := database.Get(ctx, "users", "u2")
		fmt.Printf("Updated Jane: last_name=%s, age=%v\n", updated["last_name"], updated["age"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT (DO UPDATE - specific fields only) ===")
	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id":   "u2",
		"user_name": "should_not_change",
		"f_name":    "Should",
		"last_name": "NotChange",
		"age":       100,
		"email":     "only_this@example.com", // Only this should update
	}, ConflictOptions{
		Action:       ConflictDoUpdate,
		UpdateFields: []string{"email"}, // Only update email field
	})
	if err != nil {
		fmt.Printf("InsertOnConflict DO UPDATE (specific) failed: %v\n", err)
	} else {
		updated, _ := database.Get(ctx, "users", "u2")
		fmt.Printf("Partial update: email=%s, age=%v (should still be 29)\n", updated["email"], updated["age"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT (ERROR - default behavior) ===")
	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id":   "u1", // Already exists
		"user_name": "error_test",
	}, ConflictOptions{Action: ConflictError})
	if err != nil {
		fmt.Printf("Expected error on conflict: %v\n", err)
	} else {
		fmt.Println("ERROR: Should have returned an error!")
	}

	fmt.Println("\n=== INSERT BATCH ON CONFLICT (DO NOTHING) ===")
	batchWithDuplicates := []Record{
		{"user_id": "u1", "user_name": "dup1", "f_name": "Dup", "last_name": "One", "age": 1, "email": "dup1@example.com"},       // Duplicate
		{"user_id": "u100", "user_name": "newbie", "f_name": "New", "last_name": "Bie", "age": 21, "email": "newbie@example.com"}, // New
		{"user_id": "u2", "user_name": "dup2", "f_name": "Dup", "last_name": "Two", "age": 2, "email": "dup2@example.com"},        // Duplicate
	}
	countBefore, _ := database.Count(ctx, "users")
	err = database.InsertBatchOnConflict(ctx, "users", batchWithDuplicates, ConflictOptions{Action: ConflictDoNothing})
	if err != nil {
		fmt.Printf("InsertBatchOnConflict DO NOTHING failed: %v\n", err)
	} else {
		countAfter, _ := database.Count(ctx, "users")
		fmt.Printf("Batch with duplicates: before=%d, after=%d (should be +1 for u100)\n", countBefore, countAfter)
	}

	fmt.Println("\n=== INSERT BATCH ON CONFLICT (DO UPDATE) ===")
	batchUpdate := []Record{
		{"user_id": "u100", "age": 22},                         // Update existing
		{"user_id": "u101", "user_name": "brand_new", "f_name": "Brand", "last_name": "New", "age": 30, "email": "brand@example.com"}, // Insert new
	}
	err = database.InsertBatchOnConflict(ctx, "users", batchUpdate, ConflictOptions{Action: ConflictDoUpdate})
	if err != nil {
		fmt.Printf("InsertBatchOnConflict DO UPDATE failed: %v\n", err)
	} else {
		u100, _ := database.Get(ctx, "users", "u100")
		fmt.Printf("u100 updated age: %v\n", u100["age"])
		u101, _ := database.Get(ctx, "users", "u101")
		fmt.Printf("u101 new user: %s\n", u101["user_name"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT with MergeFunc (increment age) ===")
	// First, get current age
	u100Before, _ := database.Get(ctx, "users", "u100")
	fmt.Printf("u100 age before: %v\n", u100Before["age"])

	// Define a merge function that adds to existing value
	addInt := func(existing, new any) any {
		existingInt, _ := existing.(int)
		newInt, _ := new.(int)
		return existingInt + newInt
	}

	// Insert with merge function - this will ADD 5 to the existing age
	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id": "u100",
		"age":     5, // This will be ADDED to existing age
	}, ConflictOptions{
		Action:       ConflictDoUpdate,
		UpdateFields: []string{"age"},
		MergeFuncs: map[string]MergeFunc{
			"age": addInt,
		},
	})
	if err != nil {
		fmt.Printf("InsertOnConflict with MergeFunc failed: %v\n", err)
	} else {
		u100After, _ := database.Get(ctx, "users", "u100")
		fmt.Printf("u100 age after adding 5: %v\n", u100After["age"])
	}

	fmt.Println("\n=== INSERT ON CONFLICT with MergeFunc (concatenate strings) ===")
	// Define a merge function for concatenating strings
	concatStr := func(existing, new any) any {
		existingStr, _ := existing.(string)
		newStr, _ := new.(string)
		return existingStr + newStr
	}

	u100Before, _ = database.Get(ctx, "users", "u100")
	fmt.Printf("u100 user_name before: %v\n", u100Before["user_name"])

	err = database.InsertOnConflict(ctx, "users", Record{
		"user_id":   "u100",
		"user_name": "_updated", // This will be appended
	}, ConflictOptions{
		Action:       ConflictDoUpdate,
		UpdateFields: []string{"user_name"},
		MergeFuncs: map[string]MergeFunc{
			"user_name": concatStr,
		},
	})
	if err != nil {
		fmt.Printf("InsertOnConflict with concat MergeFunc failed: %v\n", err)
	} else {
		u100After, _ := database.Get(ctx, "users", "u100")
		fmt.Printf("u100 user_name after concat: %v\n", u100After["user_name"])
	}

	fmt.Println("\n=== BATCH INSERT with MergeFunc (increment counters) ===")
	// Simulate counting occurrences - each insert increments the counter
	counterRecords := []Record{
		{"user_id": "u100", "age": 1}, // +1
		{"user_id": "u100", "age": 1}, // +1
		{"user_id": "u100", "age": 1}, // +1
	}

	u100Before, _ = database.Get(ctx, "users", "u100")
	fmt.Printf("u100 age before batch increment: %v\n", u100Before["age"])

	err = database.InsertBatchOnConflict(ctx, "users", counterRecords, ConflictOptions{
		Action:       ConflictDoUpdate,
		UpdateFields: []string{"age"},
		MergeFuncs: map[string]MergeFunc{
			"age": addInt,
		},
	})
	if err != nil {
		fmt.Printf("InsertBatchOnConflict with MergeFunc failed: %v\n", err)
	} else {
		u100After, _ := database.Get(ctx, "users", "u100")
		fmt.Printf("u100 age after batch increment (+3): %v\n", u100After["age"])
	}
}
