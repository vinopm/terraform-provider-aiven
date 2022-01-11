// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package cache

import (
	"log"
	"sync"

	aiven "github.com/aiven/aiven-go-client"
)

var (
	once       sync.Once
	topicCache *TopicCache
)

// TopicCache represents Kafka Topics cache based on Service and Project identifiers
type TopicCache struct {
	sync.RWMutex
	internal map[string]map[string]aiven.KafkaTopic
	inQueue  map[string][]string
}

// NewTopicCache creates new global instance of Kafka Topic Cache
func NewTopicCache() *TopicCache {
	log.Print("[DEBUG] Creating an instance of TopicCache ...")

	once.Do(func() {
		topicCache = &TopicCache{
			internal: make(map[string]map[string]aiven.KafkaTopic),
			inQueue:  make(map[string][]string),
		}
	})

	return topicCache
}

// GetTopicCache gets a global Kafka Topics Cache
func GetTopicCache() *TopicCache {
	return topicCache
}

// LoadByProjectAndServiceName returns a list of Kafka Topics stored in the cache for a given Project
// and Service names, or nil if no value is present.
// The ok result indicates whether value was found in the map.
func (t *TopicCache) LoadByProjectAndServiceName(projectName, serviceName string) (map[string]aiven.KafkaTopic, bool) {
	t.RLock()
	result, ok := t.internal[projectName+serviceName]
	t.RUnlock()

	return result, ok
}

// LoadByTopicName returns a list of Kafka Topics stored in the cache for a given Project
// and Service names, or nil if no value is present.
// The ok result indicates whether value was found in the map.
func (t *TopicCache) LoadByTopicName(projectName, serviceName, topicName string) (aiven.KafkaTopic, bool) {
	t.RLock()
	defer t.RUnlock()

	topics, ok := t.internal[projectName+serviceName]
	if !ok {
		return aiven.KafkaTopic{State: "CONFIGURING"}, false
	}

	result, ok := topics[topicName]
	if !ok {
		result.State = "CONFIGURING"
	}

	log.Printf("[TRACE] retrienve from a topic cache `%+#v` for a topic name `%s`", result, topicName)

	return result, ok
}

// DeleteByProjectAndServiceName deletes the cache value for a key which is a combination of Project
// and Service names.
func (t *TopicCache) DeleteByProjectAndServiceName(projectName, serviceName string) {
	t.Lock()
	delete(t.internal, projectName+serviceName)
	t.Unlock()
}

// StoreByProjectAndServiceName sets the values for a Project name and Service name key.
func (t *TopicCache) StoreByProjectAndServiceName(projectName, serviceName string, list []*aiven.KafkaTopic) {
	if len(list) == 0 {
		return
	}

	log.Printf("[DEBUG] Updating Kafka Topic cache for project %s and service %s ...", projectName, serviceName)

	for _, topic := range list {
		t.Lock()
		if _, ok := t.internal[projectName+serviceName]; !ok {
			t.internal[projectName+serviceName] = make(map[string]aiven.KafkaTopic)
		}
		t.internal[projectName+serviceName][topic.TopicName] = *topic

		// when topic is added to cache, it need to be deleted from the queue
		for i, name := range t.inQueue[projectName+serviceName] {
			if name == topic.TopicName {
				t.inQueue[projectName+serviceName] = append(t.inQueue[projectName+serviceName][:i], t.inQueue[projectName+serviceName][i+1:]...)
			}
		}

		t.Unlock()
	}
}

// IsEmpty checks if cache is empty for particular service
func (t *TopicCache) IsQueueEmpty(projectName, serviceName string) bool {
	t.RLock()
	defer t.RUnlock()

	_, ok := t.inQueue[projectName+serviceName]

	return !ok
}

// AddToQueue adds a topic name to a queue of topics to be found
func (t *TopicCache) AddToQueue(projectName, serviceName, topicName string) {
	var isFound bool

	t.Lock()
	// check if topic is already in the queue
	for _, name := range t.inQueue[projectName+serviceName] {
		if name == topicName {
			isFound = true
		}
	}

	_, inCache := t.internal[projectName+serviceName][topicName]
	// the only topic that is not in the queue nor inside cache can be added to the queue
	if !isFound && !inCache {
		t.inQueue[projectName+serviceName] = append(t.inQueue[projectName+serviceName], topicName)
	}
	t.Unlock()
}

// GetQueue retrieves a topics queue, retrieves up to 100 first elements
func (t *TopicCache) GetQueue(projectName, serviceName string) []string {
	t.RLock()
	defer t.RUnlock()

	if len(t.inQueue[projectName+serviceName]) >= 100 {
		return t.inQueue[projectName+serviceName][:99]
	}

	return t.inQueue[projectName+serviceName]
}
