package suite_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/client"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/config"
	"github.com/trustification/trusted-profile-analyzer-operator/pkg/tlsconfigurator/controller"
)

func TestTLSConfigurator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TLS Configurator Suite")
}

var _ = Describe("TLS Configuration", func() {
	Context("Config package", func() {
		It("should create a new config with defaults", func() {
			cfg := config.NewConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.IngressControllerName).To(Equal("default"))
			Expect(cfg.Namespace).To(Equal("openshift-ingress-operator"))
		})

		It("should validate config correctly", func() {
			cfg := &config.Config{
				IngressControllerName: "test",
				Namespace:             "test-namespace",
			}
			err := cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail validation with empty ingress controller name", func() {
			cfg := &config.Config{
				IngressControllerName: "",
				Namespace:             "test-namespace",
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
		})

		It("should build TLS profile correctly", func() {
			tlsConfig := &config.TLSConfig{
				Type:          configv1.TLSProfileCustomType,
				Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
				MinTLSVersion: configv1.VersionTLS13,
			}

			profile := config.BuildTLSProfile(tlsConfig)
			Expect(profile).NotTo(BeNil())
			Expect(profile.Type).To(Equal(configv1.TLSProfileCustomType))
			Expect(profile.Custom).NotTo(BeNil())
			Expect(profile.Custom.MinTLSVersion).To(Equal(configv1.VersionTLS13))
			Expect(profile.Custom.Ciphers).To(ContainElement("TLS_AES_128_GCM_SHA256"))
		})
	})

	Context("Client validation", func() {
		It("should validate custom TLS profile with TLS 1.3", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			err := client.ValidateTLSProfile(profile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject invalid TLS profile", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				// Missing Custom field
			}

			err := client.ValidateTLSProfile(profile)
			Expect(err).To(HaveOccurred())
		})

		It("should validate predefined profiles", func() {
			profiles := []configv1.TLSProfileType{
				configv1.TLSProfileIntermediateType,
				configv1.TLSProfileModernType,
				configv1.TLSProfileOldType,
			}

			for _, profileType := range profiles {
				profile := &configv1.TLSSecurityProfile{
					Type: profileType,
				}
				err := client.ValidateTLSProfile(profile)
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})

	Context("Controller operations", func() {
		It("should compare TLS profiles correctly", func() {
			profile1 := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			profile2 := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			different := controller.CompareTLSProfiles(profile1, profile2)
			Expect(different).To(BeFalse())
		})

		It("should detect different TLS profiles", func() {
			profile1 := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			profile2 := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_256_GCM_SHA384"},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			different := controller.CompareTLSProfiles(profile1, profile2)
			Expect(different).To(BeTrue())
		})
	})

	Context("Integration scenarios", func() {
		It("should create a complete TLS configuration workflow", func() {
			// Create config
			cfg := config.NewConfig()
			cfg.IngressControllerName = "test-ingress"
			cfg.Namespace = "test-namespace"

			// Validate config
			err := cfg.Validate()
			Expect(err).ToNot(HaveOccurred())

			// Build TLS configuration
			tlsConfig := &config.TLSConfig{
				Type:          configv1.TLSProfileCustomType,
				Ciphers:       []string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
				MinTLSVersion: configv1.VersionTLS13,
			}

			// Build profile
			profile := config.BuildTLSProfile(tlsConfig)
			Expect(profile).NotTo(BeNil())

			// Validate profile
			err = client.ValidateTLSProfile(profile)
			Expect(err).ToNot(HaveOccurred())

			// Verify all fields
			Expect(profile.Type).To(Equal(configv1.TLSProfileCustomType))
			Expect(profile.Custom.MinTLSVersion).To(Equal(configv1.VersionTLS13))
			Expect(profile.Custom.Ciphers).To(HaveLen(2))
		})

		It("should handle multiple TLS versions", func() {
			versions := []configv1.TLSProtocolVersion{
				configv1.VersionTLS10,
				configv1.VersionTLS11,
				configv1.VersionTLS12,
				configv1.VersionTLS13,
			}

			for _, version := range versions {
				tlsConfig := &config.TLSConfig{
					Type:          configv1.TLSProfileCustomType,
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: version,
				}

				profile := config.BuildTLSProfile(tlsConfig)
				err := client.ValidateTLSProfile(profile)
				Expect(err).ToNot(HaveOccurred())
				Expect(profile.Custom.MinTLSVersion).To(Equal(version))
			}
		})
	})
})

var _ = Describe("Edge Cases", func() {
	Context("Nil handling", func() {
		It("should handle nil TLS config gracefully", func() {
			profile := config.BuildTLSProfile(nil)
			Expect(profile).To(BeNil())
		})

		It("should reject nil TLS profile in validation", func() {
			err := client.ValidateTLSProfile(nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Empty values", func() {
		It("should reject empty ciphers list", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{},
						MinTLSVersion: configv1.VersionTLS13,
					},
				},
			}

			err := client.ValidateTLSProfile(profile)
			Expect(err).To(HaveOccurred())
		})

		It("should reject empty min TLS version", func() {
			profile := &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
						MinTLSVersion: "",
					},
				},
			}

			err := client.ValidateTLSProfile(profile)
			Expect(err).To(HaveOccurred())
		})
	})
})

// Benchmark tests
var _ = Describe("Performance", func() {
	Measure("TLS profile building performance", func(b Benchmarker) {
		runtime := b.Time("runtime", func() {
			for i := 0; i < 1000; i++ {
				tlsConfig := &config.TLSConfig{
					Type:          configv1.TLSProfileCustomType,
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				}
				config.BuildTLSProfile(tlsConfig)
			}
		})

		Expect(runtime.Seconds()).To(BeNumerically("<", 1.0), "should build 1000 profiles in less than 1 second")
	}, 5)

	Measure("TLS profile validation performance", func(b Benchmarker) {
		profile := &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"TLS_AES_128_GCM_SHA256"},
					MinTLSVersion: configv1.VersionTLS13,
				},
			},
		}

		runtime := b.Time("runtime", func() {
			for i := 0; i < 1000; i++ {
				_ = client.ValidateTLSProfile(profile)
			}
		})

		Expect(runtime.Seconds()).To(BeNumerically("<", 1.0), "should validate 1000 profiles in less than 1 second")
	}, 5)
})
