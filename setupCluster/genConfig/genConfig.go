package main

import (
  "flag"
  "fmt"
  "gopkg.in/yaml.v2"
  "io/ioutil"
  "log"
  "path/filepath"
  "strconv"
  "strings"
  "time"
)

// TopLevel consists of the structs used by the configtxgen tool.
type TopLevel struct {
  Profiles      map[string]*Profile `yaml:"Profiles,omitempty"`
  Organizations []*Organization     `yaml:"Organizations,omitempty"`
  Application   *Application        `yaml:"Application,omitempty"`
  Orderer       *Orderer            `yaml:"Orderer,omitempty"`
}

// Profile encodes orderer/application configuration combinations for the configtxgen tool.
type Profile struct {
  Consortium  string                 `yaml:"Consortium,omitempty"`
  Application *Application           `yaml:"Application,omitempty"`
  Orderer     *Orderer               `yaml:"Orderer,omitempty"`
  Consortiums map[string]*Consortium `yaml:"Consortiums,omitempty"`
}

// Consortium represents a group of organizations which may create channels with eachother
type Consortium struct {
  Organizations []*Organization `yaml:"Organizations,omitempty"`
}

// Application encodes the application-level configuration needed in config transactions.
type Application struct {
  Organizations []*Organization `yaml:"Organizations,omitempty"`
}

// Organization encodes the organization-level configuration needed in config transactions.
type Organization struct {
  Name           string `yaml:"Name,omitempty"`
  ID             string `yaml:"ID,omitempty"`
  MSPDir         string `yaml:"MSPDir,omitempty"`
  AdminPrincipal string `yaml:"AdminPrincipal,omitempty"`

  // Note: Viper deserialization does not seem to care for
  // embedding of types, so we use one organization struct
  // for both orderers and applications.
  AnchorPeers []*AnchorPeer `yaml:"AnchorPeers,omitempty"`
}

// AnchorPeer encodes the necessary fields to identify an anchor peer.
type AnchorPeer struct {
  Host string `yaml:"Host,omitempty"`
  Port int    `yaml:"Port,omitempty"`
}

// ApplicationOrganization ...
// TODO This should probably be removed
type ApplicationOrganization struct {
  Organization `yaml:"Organization,omitempty"`
}

// Orderer contains configuration which is used for the
// bootstrapping of an orderer by the provisional bootstrapper.
type Orderer struct {
  OrdererType   string          `yaml:"OrdererType,omitempty"`
  Addresses     []string        `yaml:"Addresses,omitempty"`
  BatchTimeout  time.Duration   `yaml:"BatchTimeout,omitempty"`
  BatchSize     BatchSize       `yaml:"BatchSize,omitempty"`
  Kafka         Kafka           `yaml:"Kafka,omitempty"`
  Organizations []*Organization `yaml:"Organizations,omitempty"`
  MaxChannels   uint64          `yaml:"MaxChannels,omitempty"`
}

// BatchSize contains configuration affecting the size of batches.
type BatchSize struct {
  MaxMessageCount   uint32 `yaml:"MaxMessageCount,omitempty"`
  AbsoluteMaxBytes  uint32 `yaml:"AbsoluteMaxBytes,omitempty"`
  PreferredMaxBytes uint32 `yaml:"PreferredMaxBytes,omitempty"`
}

// Kafka contains configuration for the Kafka-based orderer.
type Kafka struct {
  Brokers []string `yaml:"Brokers"`
}

// config
type Conf struct {
  Tenant      string        `yaml:"Tenant"`
  OrdererOrgs []*OrdererOrg `yaml:"OrdererOrgs"`
  PeerOrgs    []*PeerOrg    `yaml:"PeerOrgs"`
}

type PeerOrg struct {
  Name     string   `yaml:"Name"`
  Domain   string   `yaml:"Domain"`
  Template Template `yaml:"Template"`
  Users    Template `yaml:"Users"`
}

type OrdererOrg struct {
  Name     string   `yaml:"Name"`
  Domain   string   `yaml:"Domain"`
  Template Template `yaml:"Template"`
}

type Template struct {
  Count int `yaml:"Count"`
}

func GenConfigtx(conf Conf) (TopLevel, error) {

  var orderer Orderer
  orderer, _ = GenOrderer(conf)

  var orgs []*Organization
  for _, org := range conf.PeerOrgs {
    temporg, _ := GenOrg(org, conf.Tenant)
    orgs = append(orgs, &temporg)
  }

  conList := make(map[string]*Consortium, 1)
  conList["SampleConsortium"] = &Consortium{
    Organizations: orgs,
  }

  profGenesis := Profile{
    Orderer:     &orderer,
    Consortiums: conList,
  }

  profChannel := Profile{
    Consortium: "SampleConsortium",
    Application: &Application{
      Organizations: orgs,
    },
  }

  topProfile := make(map[string]*Profile, 2)
  topProfile["MultiOrgsOrdererGenesis"] = &profGenesis
  topProfile["MultiOrgsChannel"] = &profChannel

  domainName := conf.OrdererOrgs[0].Domain + conf.Tenant
  topOrg := make([]*Organization, len(orgs)+1)
  topOrg = append([]*Organization{GenOrdererOrg(domainName)}, orgs...)

  topOrderer := &orderer

  topLevel := TopLevel{
    Profiles:      topProfile,
    Organizations: topOrg,
    Orderer:       topOrderer,
  }

  return topLevel, nil
}

func GenOrg(peerOrg *PeerOrg, tenant string) (Organization, error) {

  var anchors []*AnchorPeer
  domainName := peerOrg.Domain + "-" + tenant

  for i := 0; i < peerOrg.Template.Count; i++ {
    anchor := AnchorPeer{
      Host: "peer" + strconv.Itoa(i) + "." + domainName,
      Port: 7051,
    }

    anchors = append(anchors, &anchor)
  }

  // set msp, force Capitialize
  mspID := strings.Title(strings.ToLower(peerOrg.Name)) + "MSP"
  fmt.Println(mspID)
  org := Organization{
    Name:        mspID,
    ID:          mspID,
    MSPDir:      "crypto-config/peerOrganizations/" + domainName + "/msp",
    AnchorPeers: anchors,
  }

  return org, nil
}

// by default it is the first orderer that has msp
func GenOrdererOrg(domainName string) *Organization {
  ordererOrg := Organization{
    Name:   "OrdererOrg",
    ID:     "Orderer0MSP",
    MSPDir: "crypto-config/ordererOrganizations/" + domainName + "/msp",
  }
  //orderer.Organizations[0] = &ordererOrg

  return &ordererOrg
}

func GenKafka(conf Conf) (Kafka, error) {
  var kafka_list []string
  for i := 0; i < len(conf.OrdererOrgs); i++ {
    kafka_list = append(kafka_list, "kafka"+strconv.Itoa(i)+"."+conf.OrdererOrgs[i].Domain+"-"+conf.Tenant+":9092")
  }

  var kafka = Kafka{
    Brokers: kafka_list,
  }

  return kafka, nil
}

func GenOrdererDomain(conf Conf, index int) string {
  return "orderer" + strconv.Itoa(index) + "." + conf.OrdererOrgs[index].Domain + "-" + conf.Tenant + ":7050"
}

func GenOrderer(conf Conf) (Orderer, error) {
  var address_list []string
  var orderer Orderer
  if len(conf.OrdererOrgs) == 1 {
    address_list = append(address_list, GenOrdererDomain(conf, 0))
    orderer = Orderer{
      OrdererType:  "solo",
      Addresses:    address_list,
      BatchTimeout: 2 * time.Second,
      BatchSize: BatchSize{
        MaxMessageCount:   10,
        AbsoluteMaxBytes:  99 * 1024 * 1024, // 99 MB
        PreferredMaxBytes: 512 * 1024,       // 512 KB
      },
      Organizations: make([]*Organization, 1),
    }
  } else {
    numOrderer := len(conf.OrdererOrgs)
    for i := 0; i < numOrderer; i++ {
      address_list = append(address_list, GenOrdererDomain(conf, i))
    }

    kafka, _ := GenKafka(conf)

    orderer = Orderer{
      OrdererType:  "kafka",
      Addresses:    address_list,
      BatchTimeout: 2 * time.Second,
      BatchSize: BatchSize{
        MaxMessageCount:   10,
        AbsoluteMaxBytes:  99 * 1024 * 1024, // 99 MB
        PreferredMaxBytes: 512 * 1024,       // 512 KB
      },
      Kafka:         kafka,
      Organizations: make([]*Organization, 1),
    }
  }
  domainName := conf.OrdererOrgs[0].Domain + "-" + conf.Tenant
  orderer.Organizations[0] = GenOrdererOrg(domainName)
  return orderer, nil
}

func getConf(path string) Conf {

  yamlFile, err := ioutil.ReadFile(path)
  if err != nil {
    log.Printf("yamlFile.Get err   #%v ", err)
  }
  var c Conf
  err = yaml.Unmarshal(yamlFile, &c)
  if err != nil {
    log.Fatalf("Unmarshal: %v", err)
  }

  return c
}

func main() {

  var configPath, outputPath string
  flag.StringVar(&configPath, "In", "../cluster-config.yaml", "Config path of the network")
  flag.StringVar(&outputPath, "Out", "../configtx.yaml", "Config path of the network")
  // for kafka zoo keeper to work, you have to use kafka-zookeeper template
  // and config it by-hand

  flag.Parse()

  absConfigPath, err := filepath.Abs(configPath)
  check(err)

  conf := getConf(absConfigPath)

  absOutputPath, err := filepath.Abs(outputPath)
  check(err)

  // Generate configtx.yaml
  configtx, err := GenConfigtx(conf)
  check(err)
  fmt.Println("Generating YAML file from configtx config....")
  configtxYAML, err := yaml.Marshal(&configtx)
  check(err)

  // Write files to $PWD

  err = ioutil.WriteFile(absOutputPath, []byte(configtxYAML), 0644)
  check(err)

  fmt.Println("Output files are located on " + absOutputPath)
}

func check(e error) {
  if e != nil {
    panic(e)
  }
}