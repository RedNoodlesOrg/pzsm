// Package steam is a client for the Steam Web API endpoints used to resolve
// Project Zomboid mod workshop collections.
package steam

// WorkshopFileType matches the Steam enum for published-file kinds.
type WorkshopFileType int

const (
	WorkshopFileCommunity  WorkshopFileType = 0
	WorkshopFileCollection WorkshopFileType = 2
)

const resultOK = 1

// PublishedFile is the minimal child entry returned inside a CollectionDetails.
type PublishedFile struct {
	PublishedFileID string           `json:"publishedfileid"`
	SortOrder       int              `json:"sortorder"`
	FileType        WorkshopFileType `json:"filetype"`
}

// CollectionDetails describes one workshop collection and its children.
type CollectionDetails struct {
	Result          int             `json:"result"`
	PublishedFileID string          `json:"publishedfileid"`
	Children        []PublishedFile `json:"children"`
}

// PublishedFileDetails describes one workshop item (a mod, in our case).
type PublishedFileDetails struct {
	PublishedFileID string `json:"publishedfileid"`
	Result          int    `json:"result"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	PreviewURL      string `json:"preview_url"`
}

type collectionDetailsResponse struct {
	Result            int                 `json:"result"`
	ResultCount       int                 `json:"resultcount"`
	CollectionDetails []CollectionDetails `json:"collectiondetails"`
}

type publishedFileDetailsResponse struct {
	Result               int                    `json:"result"`
	ResultCount          int                    `json:"resultcount"`
	PublishedFileDetails []PublishedFileDetails `json:"publishedfiledetails"`
}

type envelope[T any] struct {
	Response T `json:"response"`
}
