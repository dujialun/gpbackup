package integration

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/greenplum-db/gp-common-go-libs/operating"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	testDir          = "/tmp/helper_test"
	pluginDir        = "/tmp/plugin_dest"
	tocFile          = fmt.Sprintf("%s/test_toc.yaml", testDir)
	oidFile          = fmt.Sprintf("%s/test_oids", testDir)
	pipeFile         = fmt.Sprintf("%s/test_pipe", testDir)
	dataFileFullPath = filepath.Join(testDir, "test_data")
	pluginBackupPath = filepath.Join(pluginDir, "test_data")
	errorFile        = fmt.Sprintf("%s_error", pipeFile)
	pluginConfigPath = fmt.Sprintf("%s/go/src/github.com/greenplum-db/gpbackup/plugins/example_plugin_config.yaml", os.Getenv("HOME"))
)

const (
	expectedData = `here is some data
here is some data
here is some data
`
	expectedTOC = `dataentries:
  1:
    startbyte: 0
    endbyte: 18
  2:
    startbyte: 18
    endbyte: 36
  3:
    startbyte: 36
    endbyte: 54
`
)

func gpbackupHelper(helperPath string, args ...string) *exec.Cmd {
	args = append([]string{"--toc-file", tocFile, "--oid-file", oidFile, "--pipe-file", pipeFile, "--content", "1"}, args...)
	command := exec.Command(helperPath, args...)
	err := command.Start()
	Expect(err).ToNot(HaveOccurred())
	return command
}

func buildAndInstallBinaries() string {
	os.Chdir("..")
	command := exec.Command("make", "build")
	output, err := command.CombinedOutput()
	if err != nil {
		fmt.Printf("%s", output)
		Fail(fmt.Sprintf("%v", err))
	}
	os.Chdir("integration")
	binDir := fmt.Sprintf("%s/go/bin", operating.System.Getenv("HOME"))
	return fmt.Sprintf("%s/gpbackup_helper", binDir)
}

var _ = Describe("gpbackup_helper end to end integration tests", func() {
	BeforeEach(func() {
		err := os.RemoveAll(testDir)
		Expect(err).ToNot(HaveOccurred())
		err = os.MkdirAll(testDir, 0777)
		Expect(err).ToNot(HaveOccurred())
		err = os.RemoveAll(pluginDir)
		Expect(err).ToNot(HaveOccurred())
		err = os.MkdirAll(pluginDir, 0777)
		Expect(err).ToNot(HaveOccurred())

		err = syscall.Mkfifo(fmt.Sprintf("%s_%d", pipeFile, 1), 0777)
		if err != nil {
			Fail(fmt.Sprintf("%v", err))
		}
	})
	Context("backup tests", func() {
		BeforeEach(func() {
			f, _ := os.Create(oidFile)
			f.WriteString("1\n2\n3\n")
		})
		It("runs backup gpbackup_helper without compression", func() {
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--backup-agent", "--compression-level", "0", "--data-file", dataFileFullPath)
			writeToPipes()
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertBackupArtifacts(false, false)
		})
		It("runs backup gpbackup_helper with compression", func() {
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--backup-agent", "--compression-level", "1", "--data-file", dataFileFullPath+".gz")
			writeToPipes()
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertBackupArtifacts(true, false)
		})
		It("runs backup gpbackup_helper without compression with plugin", func() {
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--backup-agent", "--compression-level", "0", "--data-file", dataFileFullPath, "--plugin-config", pluginConfigPath)
			writeToPipes()
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertBackupArtifacts(false, true)
		})
		It("runs backup gpbackup_helper with compression with plugin", func() {
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--backup-agent", "--compression-level", "1", "--data-file", dataFileFullPath+".gz", "--plugin-config", pluginConfigPath)
			writeToPipes()
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertBackupArtifacts(true, true)
		})
		It("Generates error file when backup agent interrupted", func() {
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--backup-agent", "--compression-level", "0", "--data-file", dataFileFullPath)
			time.Sleep(200 * time.Millisecond)
			err := helperCmd.Process.Signal(os.Interrupt)
			Expect(err).ToNot(HaveOccurred())
			err = helperCmd.Wait()
			Expect(err).To(HaveOccurred())
			assertErrorsHandled()
		})
	})
	Context("restore tests", func() {
		It("runs restore gpbackup_helper without compression", func() {
			setupRestoreFiles(false, false)
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--restore-agent", "--data-file", dataFileFullPath)
			for _, i := range []int{1, 3} {
				contents, _ := ioutil.ReadFile(fmt.Sprintf("%s_%d", pipeFile, i))
				Expect(string(contents)).To(Equal("here is some data\n"))
			}
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertNoErrors()
		})
		It("runs restore gpbackup_helper with compression", func() {
			setupRestoreFiles(true, false)
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--restore-agent", "--data-file", dataFileFullPath+".gz")
			for _, i := range []int{1, 3} {
				contents, _ := ioutil.ReadFile(fmt.Sprintf("%s_%d", pipeFile, i))
				Expect(string(contents)).To(Equal("here is some data\n"))
			}
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertNoErrors()
		})
		It("runs restore gpbackup_helper without compression with plugin", func() {
			setupRestoreFiles(false, true)
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--restore-agent", "--data-file", dataFileFullPath, "--plugin-config", pluginConfigPath)
			for _, i := range []int{1, 3} {
				contents, _ := ioutil.ReadFile(fmt.Sprintf("%s_%d", pipeFile, i))
				Expect(string(contents)).To(Equal("here is some data\n"))
			}
			err := helperCmd.Wait()
			printHelperLogOnError(err)
			Expect(err).ToNot(HaveOccurred())
			assertNoErrors()
		})
		It("runs restore gpbackup_helper with compression with plugin", func() {
			setupRestoreFiles(true, true)
			gpbackupHelper(gpbackupHelperPath, "--restore-agent", "--data-file", dataFileFullPath+".gz", "--plugin-config", pluginConfigPath)
			for _, i := range []int{1, 3} {
				contents, _ := ioutil.ReadFile(fmt.Sprintf("%s_%d", pipeFile, i))
				Expect(string(contents)).To(Equal("here is some data\n"))
			}
			assertNoErrors()
		})
		It("Generates error file when restore agent interrupted", func() {
			setupRestoreFiles(true, false)
			helperCmd := gpbackupHelper(gpbackupHelperPath, "--restore-agent", "--data-file", dataFileFullPath+".gz")
			time.Sleep(200 * time.Millisecond)
			err := helperCmd.Process.Signal(os.Interrupt)
			Expect(err).ToNot(HaveOccurred())
			err = helperCmd.Wait()
			Expect(err).To(HaveOccurred())
			assertErrorsHandled()
		})
	})
})

func setupRestoreFiles(withCompression bool, withPlugin bool) {
	dataFile := dataFileFullPath
	if withPlugin {
		dataFile = pluginBackupPath
	}
	f, _ := os.Create(oidFile)
	f.WriteString("1\n3\n")
	if withCompression {
		f, _ := os.Create(dataFile + ".gz")
		gzipf := gzip.NewWriter(f)
		defer gzipf.Close()
		gzipf.Write([]byte(expectedData))
	} else {
		f, _ := os.Create(dataFile)
		f.WriteString(expectedData)
	}

	f, _ = os.Create(tocFile)
	f.WriteString(expectedTOC)
}

func assertNoErrors() {
	Expect(errorFile).To(Not(BeARegularFile()))
	pipes, err := filepath.Glob(pipeFile + "_[1-9]*")
	Expect(err).ToNot(HaveOccurred())
	Expect(pipes).To(BeEmpty())
}

func assertErrorsHandled() {
	Expect(errorFile).To(BeARegularFile())
	pipes, err := filepath.Glob(pipeFile + "_[1-9]*")
	Expect(err).ToNot(HaveOccurred())
	Expect(pipes).To(BeEmpty())
}
func assertBackupArtifacts(withCompression bool, withPlugin bool) {
	var contents []byte
	var err error
	dataFile := dataFileFullPath
	if withPlugin {
		dataFile = pluginBackupPath
	}
	if withCompression {
		contents, err = ioutil.ReadFile(dataFile + ".gz")
		Expect(err).ToNot(HaveOccurred())
		r, _ := gzip.NewReader(bytes.NewReader(contents))
		contents, _ = ioutil.ReadAll(r)

	} else {
		contents, err = ioutil.ReadFile(dataFile)
		Expect(err).ToNot(HaveOccurred())
	}
	Expect(string(contents)).To(Equal(expectedData))

	contents, err = ioutil.ReadFile(tocFile)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(contents)).To(Equal(expectedTOC))
	assertNoErrors()
}

func printHelperLogOnError(helperErr error) {
	if helperErr != nil {
		homeDir := os.Getenv("HOME")
		helperFiles, _ := filepath.Glob(filepath.Join(homeDir, "gpAdminLogs/gpbackup_helper_*"))
		command := exec.Command("tail", "-n 20", helperFiles[len(helperFiles)-1])
		output, _ := command.CombinedOutput()
		fmt.Println(string(output))
	}
}

func writeToPipes() {
	for i := 1; i <= 3; i++ {
		currentPipe := fmt.Sprintf("%s_%d", pipeFile, i)
		// Wait until pipe exists before writing
		for {
			_, err := os.Stat(currentPipe)
			if err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		output, err := exec.Command("bash", "-c", fmt.Sprintf("echo here is some data > %s", currentPipe)).CombinedOutput()
		if err != nil {
			fmt.Printf("%s", output)
			Fail(fmt.Sprintf("%v", err))
		}
	}
}
