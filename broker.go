package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
	"fmt"
	"bytes"
	"bufio"
	"strings"
	"errors"
	"log"
	"io/ioutil"
)

type broker struct {
	config *Config
}

func newBroker(config *Config) *broker {
	return &broker{
		config: config,
	}
}

func (b *broker) FetchRemoteEntries() ([]*entry, error) {
	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	client, err := NewClient(b.config.AccessToken, logger)

	items, err := client.GetItems()
	if err != nil {
		return nil, err
	}

	var entries []*entry
	for _, i := range items {
		e := i.ConvertToEntry()

		entries = append(entries, e)
	}

	return entries, err
}

func (b *broker) FetchRemoteEntry(id string) (*entry, error) {

	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	client, err := NewClient(b.config.AccessToken, logger)
	item, err := client.GetItem(id)
	if err != nil {
		return nil, err
	}
	entry := item.ConvertToEntry()

	return entry, err
}

func (b *broker) LocalPath(e *entry) string {
	extension := ".md"
	paths := []string{b.config.LocalRoot}
	pathFormat := "2006/01/02"
	datePath := e.Date.Format(pathFormat)
	paths = append(paths, datePath)
	idPath := e.ID + extension
	paths = append(paths, idPath)

	return filepath.Join(paths...)
}

func (b *broker) StoreFresh(e *entry, path string) (bool, error) {
	var localLastModified time.Time
	if fi, err := os.Stat(path); err == nil {
		localLastModified = fi.ModTime()
	}

	if e.LastModified.After(localLastModified) {
		logf("fresh", "remote=%s > local=%s", e.LastModified, localLastModified)
		if err := b.Store(e, path); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (b *broker) Store(e *entry, path string) error {
	logf("store", "%s", path)

	dir, _ := filepath.Split(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	_, err = f.WriteString(e.fullContent())
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	return os.Chtimes(path, *e.LastModified, *e.LastModified)
}

func (b *broker) PutEntry(e *entry) error {
	i := e.ConvertToItem()
	jsonBytes, err := json.Marshal(i)
	if err != nil {
		fmt.Println(err)
		return err
	}

	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	client, err := NewClient(b.config.AccessToken, logger)

	item, err := client.PatchItem(e.ID, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}

	newEntry := item.ConvertToEntry()

	path := b.LocalPath(newEntry)
	_, err = b.StoreFresh(newEntry, path)
	if err != nil {
		return err
	}
	return nil
}

func (b *broker) UploadFresh(e *entry) (bool, error) {
	re, err := b.FetchRemoteEntry(e.ID)
	if err != nil {
		return false, err
	}

	if e.LastModified.After(*re.LastModified) == false {
		return false, nil
	}

	return true, b.PutEntry(e)
}

func convetInputToTags(s string) ([]tag, error) {
	if len(s) == 0 {
		return nil, errors.New("１つ以上５つ以下のタグをつけてください")
	}
	tagstrings := strings.Split(s, " ")

	var tags []tag
	if len(tagstrings) > 5 {
		return nil, errors.New("タグは５つまでです")
	}

	for _, tagstring := range tagstrings {
		t := strings.Split(tagstring, ":")
		if len(t) > 2 {
			return nil, errors.New("タグの形式が不正です")
		}
		tag := tag{Name: t[0], Versions: make([]string, 0)}
		if len(t) > 1 {
			tag.Versions = strings.Split(t[1], ",")
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func (b *broker) PostEntry() error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("title:")
	scanner.Scan()
	title := scanner.Text()
	if len(title) == 0 {
		return errors.New("タイトルは必須項目です(255文字以下)")
	}
	if len(title) > 255 {
		return errors.New("タイトルは255文字以下です")
	}

	fmt.Printf("tags:")
	scanner.Scan()
	t := scanner.Text()
	tags, err := convetInputToTags(t)
	if err != nil {
		return err
	}

	i := item{Title: title, Tags: tags, Private: true, Body: "#WIP"}
	jsonBytes, err := json.Marshal(i)
	if err != nil {
		return err
	}

	logger := log.New(ioutil.Discard, "", log.LstdFlags)
	client, err := NewClient(b.config.AccessToken, logger)

	item, err := client.PostItem(bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}

	newEntry := item.ConvertToEntry()
	if err != nil {
		return err
	}

	path := b.LocalPath(newEntry)
	return b.Store(newEntry, path)
}
