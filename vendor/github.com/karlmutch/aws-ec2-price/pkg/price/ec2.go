package price

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/karlmutch/aws-ec2-price/pkg/price/version"
)

const (
	currentOfferURL  = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.json"
	offerVersionsURL = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/index.json"

	hourlyTermCode = "JRTCKXETXF"
	rateCode       = "6YS6EN2CT7"

	requiredTerm           = "OnDemand"
	requiredTenancy        = "Shared"
	requiredProductFamily  = "Compute Instance"
	requiredOs             = "Linux"
	requiredLicenseModel   = "No License required"
	requiredUsage          = "BoxUsage:*"
	requiredPreinstalledSw = "NA"
)

var (
	cachedPricing = cachedEc2Pricing{
		infos: map[string]*ec2PricingInfo{},
	}
)

type ec2Pricing struct {
	Products map[string]Ec2Product `json:products`
	Terms    map[string]map[string]map[string]struct {
		PriceDimensions map[string]struct {
			PricePerUnit struct {
				USD string `json:USD`
			} `json:pricePerUnit`
		} `json:priceDimensions`
	} `json:terms`
}

// NewPricing will refresh pricing for the indiciated AWS region and will return the
// appropriate pricing table for the region that can then be used by the client
//
func NewPricing(region string) (pricing *ec2Pricing, err error) {
	if !cachedPricing.isExpired(region) {
		return cachedPricing.infos[region].pricing, nil
	}

	if pricing, err = refreshPricing(region); err != nil {
		return nil, err
	}

	cachedPricing.update(region, pricing)

	return pricing, nil
}

// GetInstances will retrieve all instance details for the specified region.  The receiver is
// initialized using a single region which will cause a cache to be refresh for that region,
// however because the cache is shared the client can request information for any region
// that might be present.
//
func (ec *ec2Pricing) GetInstances(region string) (instances []*Instance, err error) {
	instances = []*Instance{}
	for _, product := range ec.Products {
		if !product.isValid() {
			continue
		}

		if !product.isValidRegion(region) {
			continue
		}

		h := fmt.Sprintf("%s.%s", product.Sku, hourlyTermCode)
		r := fmt.Sprintf("%s.%s.%s", product.Sku, hourlyTermCode, rateCode)

		usd := ec.Terms[requiredTerm][product.Sku][h].PriceDimensions[r].PricePerUnit.USD

		price, err := strconv.ParseFloat(usd, 64)
		if err != nil {
			return nil, errors.New("usd could not be parsed")
		}

		instances = append(instances, &Instance{
			Region: region,
			Type:   product.InstanceType(),
			Price:  price,
		})
	}

	return instances, nil
}

// GetInstance will retrieve a specific instances details for the specified region.  The receiver is
// initialized using a single region which will cause a cache to be refresh for that region,
// however because the cache is shared the client can request information for any region
// that might be present.
//
func (ec *ec2Pricing) GetInstance(region string, instanceType string) (instance *Instance, err error) {
	instances, err := ec.GetInstances(region)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if instance.Type != instanceType {
			continue
		}

		return instance, nil
	}

	return nil, errors.New("supplied instance not recognized")
}

type Ec2Product struct {
	Sku           string `json:sku`
	ProductFamily string `json:productFamily`
	Attributes    struct {
		Location        string `json:location`
		InstanceType    string `json:instanceType`
		Tenancy         string `json:tenancy`
		OperatingSystem string `json:operatingSystem`
		LicenseModel    string `json:licenseModel`
		UsageType       string `json:usagetype`
		PreInstalledSw  string `json:preInstalledSw`
	}
}

func (ep *Ec2Product) InstanceType() string {
	return ep.Attributes.InstanceType
}

func (ep *Ec2Product) isValidRegion(region string) bool {
	if r, ok := Regions[region]; ok {
		return ep.Attributes.Location == r
	}

	return false
}

func (ep *Ec2Product) isValid() bool {
	if ep.ProductFamily != requiredProductFamily {
		return false
	}

	if ep.Attributes.OperatingSystem != requiredOs {
		return false
	}

	if ep.Attributes.LicenseModel != requiredLicenseModel {
		return false
	}

	if ep.Attributes.Tenancy != requiredTenancy {
		return false
	}

	if ep.Attributes.PreInstalledSw != requiredPreinstalledSw {
		return false
	}

	matched, err := regexp.MatchString(requiredUsage, ep.Attributes.UsageType)
	if err != nil || matched == false {
		return false
	}

	return true

}

type cachedEc2Pricing struct {
	infos map[string]*ec2PricingInfo
}

type ec2PricingInfo struct {
	pricing       *ec2Pricing
	lastCheckTime time.Time
}

func (c *ec2PricingInfo) updateInstance(pricing *ec2Pricing) {
	c.pricing = pricing
}

func (c *cachedEc2Pricing) isExpired(region string) bool {
	if val, ok := c.infos[region]; ok {
		return time.Since(val.lastCheckTime) > time.Duration(24*time.Hour)
	}
	return true
}

func (c *cachedEc2Pricing) update(region string, pricing *ec2Pricing) {
	if val, ok := c.infos[region]; ok {
		val.updateInstance(pricing)
		return
	}
	e := &ec2PricingInfo{
		pricing:       pricing,
		lastCheckTime: time.Now(),
	}
	c.infos[region] = e
}

func refreshPricing(region string) (pricing *ec2Pricing, err error) {
	client := &http.Client{}

	// Obtain a catalog of the known versions of offer documents
	// and explicitly select the current one
	versionResp, err := client.Get(offerVersionsURL)
	if err != nil {
		return nil, err
	}
	defer versionResp.Body.Close()

	versions := &version.Version{}
	if err := json.NewDecoder(versionResp.Body).Decode(versions); err != nil {
		return nil, err
	}

	// Get the latest version of the offer document for the chosen region
	url := fmt.Sprintf("https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/%s/%s/index.json", versions.CurrentVersion, region)

	currentResp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer currentResp.Body.Close()

	pricing = &ec2Pricing{}
	if err := json.NewDecoder(currentResp.Body).Decode(pricing); err != nil {
		return nil, err
	}
	return pricing, nil
}
