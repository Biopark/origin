package images

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"

	registryutil "github.com/openshift/origin/test/extended/registry/util"
	exutil "github.com/openshift/origin/test/extended/util"
	testutil "github.com/openshift/origin/test/util"
)

const (
	testImageSize     = 1024
	mirrorBlobTimeout = time.Second * 10
	// this image has a high number of relatively small blobs
	externalImageReference = "docker.io/openshift/origin-release:golang-1.4"
)

var _ = g.Describe("[Feature:ImagePrune] Image prune", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("prune-images", exutil.KubeConfigPath())

	var originalAcceptSchema2 *bool

	g.JustBeforeEach(func() {
		if originalAcceptSchema2 == nil {
			accepts, err := registryutil.DoesRegistryAcceptSchema2(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			originalAcceptSchema2 = &accepts
		}

		err := exutil.WaitForBuilderAccount(oc.KubeREST().ServiceAccounts(oc.Namespace()))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("give a user %s a right to prune images with %s role", oc.Username(), "system:image-pruner"))
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.Describe("of schema 1", func() {
		g.JustBeforeEach(func() {
			if *originalAcceptSchema2 {
				g.By("ensure the registry does not accept schema 2")
				err := registryutil.EnsureRegistryAcceptsSchema2(oc, false)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		})

		g.AfterEach(func() {
			if *originalAcceptSchema2 {
				err := registryutil.EnsureRegistryAcceptsSchema2(oc, true)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		})

		g.It("should prune old image", func() { testPruneImages(oc, 1) })
	})

	g.Describe("of schema 2", func() {
		g.JustBeforeEach(func() {
			if !*originalAcceptSchema2 {
				g.By("ensure the registry accepts schema 2")
				err := registryutil.EnsureRegistryAcceptsSchema2(oc, true)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		})

		g.AfterEach(func() {
			if !*originalAcceptSchema2 {
				err := registryutil.EnsureRegistryAcceptsSchema2(oc, false)
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		})

		g.It("should prune old image with config", func() { testPruneImages(oc, 2) })
	})
})

func testPruneImages(oc *exutil.CLI, schemaVersion int) {
	var mediaType string
	switch schemaVersion {
	case 1:
		mediaType = schema1.MediaTypeManifest
	case 2:
		mediaType = schema2.MediaTypeManifest
	default:
		g.Fail(fmt.Sprintf("unexpected schema version %d", schemaVersion))
	}

	isName := "prune"
	repoName := oc.Namespace() + "/" + isName

	oc.SetOutputDir(exutil.TestContext.OutputDir)
	outSink := g.GinkgoWriter

	cleanUp := NewCleanUpContainer(oc)
	defer cleanUp.Run()

	dClient, err := testutil.NewDockerClient()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By(fmt.Sprintf("build two images using Docker and push them as schema %d", schemaVersion))
	imgPruneName, _, err := BuildAndPushImageOfSizeWithDocker(oc, dClient, isName, "latest", testImageSize, 2, outSink, true, true)
	o.Expect(err).NotTo(o.HaveOccurred())
	cleanUp.AddImage(imgPruneName, "", "")
	cleanUp.AddImageStream(isName)
	pruneSize, err := registryutil.GetRegistryStorageSize(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	imgKeepName, _, err := BuildAndPushImageOfSizeWithDocker(oc, dClient, isName, "latest", testImageSize, 2, outSink, true, true)
	o.Expect(err).NotTo(o.HaveOccurred())
	cleanUp.AddImage(imgKeepName, "", "")
	keepSize, err := registryutil.GetRegistryStorageSize(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(pruneSize < keepSize).To(o.BeTrue())

	g.By(fmt.Sprintf("ensure uploaded image is of schema %d", schemaVersion))
	imgPrune, err := oc.AsAdmin().REST().Images().Get(imgPruneName)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(imgPrune.DockerImageManifestMediaType).To(o.Equal(mediaType))
	imgKeep, err := oc.AsAdmin().REST().Images().Get(imgKeepName)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(imgKeep.DockerImageManifestMediaType).To(o.Equal(mediaType))

	g.By("prune the first image uploaded (dry-run)")
	output, err := oc.WithoutNamespace().Run("adm").Args("prune", "images", "--keep-tag-revisions=1", "--keep-younger-than=0").Output()

	g.By("verify images, layers and configs about to be pruned")
	o.Expect(output).To(o.ContainSubstring(imgPruneName))
	if schemaVersion == 1 {
		o.Expect(output).NotTo(o.ContainSubstring(imgPrune.DockerImageMetadata.ID))
	} else {
		o.Expect(output).To(o.ContainSubstring(imgPrune.DockerImageMetadata.ID))
	}
	for _, layer := range imgPrune.DockerImageLayers {
		if !strings.Contains(output, layer.Name) {
			o.Expect(output).To(o.ContainSubstring(layer.Name))
		}
	}

	o.Expect(output).NotTo(o.ContainSubstring(imgKeepName))
	o.Expect(output).NotTo(o.ContainSubstring(imgKeep.DockerImageMetadata.ID))
	for _, layer := range imgKeep.DockerImageLayers {
		if !strings.Contains(output, layer.Name) {
			o.Expect(output).NotTo(o.ContainSubstring(layer.Name))
		}
	}

	noConfirmSize, err := registryutil.GetRegistryStorageSize(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(noConfirmSize).To(o.Equal(keepSize))

	g.By("prune the first image uploaded (confirm)")
	output, err = oc.WithoutNamespace().Run("adm").Args("prune", "images", "--keep-tag-revisions=1", "--keep-younger-than=0", "--confirm").Output()

	g.By("verify images, layers and configs about to be pruned")
	o.Expect(output).To(o.ContainSubstring(imgPruneName))
	if schemaVersion == 1 {
		o.Expect(output).NotTo(o.ContainSubstring(imgPrune.DockerImageMetadata.ID))
	} else {
		o.Expect(output).To(o.ContainSubstring(imgPrune.DockerImageMetadata.ID))
	}
	for _, layer := range imgPrune.DockerImageLayers {
		if !strings.Contains(output, layer.Name) {
			o.Expect(output).To(o.ContainSubstring(layer.Name))
		}
		globally, inRepository, err := IsBlobStoredInRegistry(oc, digest.Digest(layer.Name), repoName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(globally).To(o.BeFalse())
		o.Expect(inRepository).To(o.BeFalse())
	}

	o.Expect(output).NotTo(o.ContainSubstring(imgKeepName))
	o.Expect(output).NotTo(o.ContainSubstring(imgKeep.DockerImageMetadata.ID))
	for _, layer := range imgKeep.DockerImageLayers {
		if !strings.Contains(output, layer.Name) {
			o.Expect(output).NotTo(o.ContainSubstring(layer.Name))
		}
		globally, inRepository, err := IsBlobStoredInRegistry(oc, digest.Digest(layer.Name), repoName)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(globally).To(o.BeTrue())
		o.Expect(inRepository).To(o.BeTrue())
	}

	confirmSize, err := registryutil.GetRegistryStorageSize(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	g.By(fmt.Sprintf("confirming storage size: sizeOfKeepImage=%d <= sizeAfterPrune=%d < beforePruneSize=%d", imgKeep.DockerImageMetadata.Size, confirmSize, keepSize))
	o.Expect(confirmSize >= imgKeep.DockerImageMetadata.Size).To(o.BeTrue())
	o.Expect(confirmSize < keepSize).To(o.BeTrue())
	g.By(fmt.Sprintf("confirming pruned size: sizeOfPruneImage=%d <= (sizeAfterPrune=%d - sizeBeforePrune=%d)", imgPrune.DockerImageMetadata.Size, keepSize, confirmSize))
	o.Expect(imgPrune.DockerImageMetadata.Size <= keepSize-confirmSize).To(o.BeTrue())
}
