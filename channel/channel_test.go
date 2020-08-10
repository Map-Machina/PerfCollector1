package channel

import (
	"context"
	"sync"
	"testing"
)

func TestStruct(t *testing.T) {
	type moo struct {
		x int
	}

	var wg sync.WaitGroup
	ctx := context.TODO()
	mC := make(chan *moo)

	wg.Add(1)
	go func() {
		defer wg.Done()
		x, err := Read(ctx, mC)
		if err != nil {
			panic(err)
		}
		if x == nil {
			t.Fatalf("expected not nil %v", x)
		}
		xx := x.(*moo)
		if xx.x != 42 {
			t.Fatalf("expected 42, got %v", xx.x)
		}
	}()

	err := Write(ctx, mC, &moo{x: 42})
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}

func TestNilStruct(t *testing.T) {
	type moo struct {
		x int
	}

	var wg sync.WaitGroup
	ctx := context.TODO()
	mC := make(chan *moo)

	wg.Add(1)
	go func() {
		defer wg.Done()
		x, err := Read(ctx, mC)
		if err != nil {
			panic(err)
		}
		if x != nil {
			t.Fatalf("expected nil %T %v", x, x)
		}
	}()

	err := Write(ctx, mC, nil)
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}

func TestNilError(t *testing.T) {
	var wg sync.WaitGroup
	ctx := context.TODO()
	mC := make(chan error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		x, err := Read(ctx, mC)
		if err != nil {
			panic(err)
		}
		if x != nil {
			t.Fatal("expected nil")
		}
	}()

	err := Write(ctx, mC, nil)
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}
