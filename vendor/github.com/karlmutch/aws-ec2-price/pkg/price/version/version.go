package version

type Item struct {
	VersionEffectiveBeginDate string `json:"versionEffectiveBeginDate"`
	VersionEffectiveEndDate string `json:"versionEffectiveEndDate"`
	OfferVersionUrl string `json:"offerVersionUrl"`
}

type Version struct {
	PublicationDate string `json:"publicationDate"`
	OfferCode string `json:"offerCode"`
	CurrentVersion string `json:"currentVersion"`
	Versions map[string]Item `json:"versions"`
}
