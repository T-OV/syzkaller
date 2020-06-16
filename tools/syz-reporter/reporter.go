// Copyright 2020 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// syz-reprorunner compiles and runs creprog on multiple VMs. Usage:
//   syz-reprorunner -config=config.json creprog.c
// creprogs can be found example from here: https://github.com/dvyukov/syzkaller-repros
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/syzkaller/pkg/log"
	//"github.com/google/syzkaller/pkg/report"
)

var (
	flagInDir 		= flag.String("input_dir", "", "Folder that contains syzrepro output")
	flagOutFile 	= flag.String("output_file", "", "Output file for the report (generated with MD markup)")
	flagCBisctDir 	= flag.String("config_bisect_dir", "", "[Locked] Directory containing config bisect results")
	// TODO: Check and make as in original report.py (unlimited number can be given?)
	flagCmpDir		= flag.String("comparison_dir", "", "Folder to be searched for reproducer results to be added into report") 


	statuses = []string {"crashed", "failed", "timed_out", "passed"}
)


func usage() {
	fmt.Fprintf(os.Stderr, "usage: report.py [-h] --input_dir INPUT_DIR --output_file OUTPUT_FILE\n\n" +
	"Generate report on syzrepro ouput dir\n\n" +
	"optional arguments:\n" +
	"  -h, --help            Show this help message and exit\n" +
	"  --input_dir INPUT_DIR\n" +
	"						Folder that contains syzrepro output\n" +
	"  --output_file OUTPUT_FILE\n" +
	"						Output file for the report.\n" +
	"						(Is generated in MD markup, so .md extension is helpul.)\n" +
	"  [ --config_bisect_dir BISECTION_DIRECTORY ]  |!Switched off until inclusion of bisection into Syz-master!| \n" +
	"						Directory containing config bisect results.\n" +
	"						(If reproducer checked before with results saved in INPUT_DIR was used\n" +
	"						also in config bisection testruns and results of the later also important to report.)\n" +
	"  [ --comparison_dir COMPARISON_DIRECTORY [COMPARISON_DIR ...], -d COMPARISON_DIR [COMPARISON_DIR ...]]\n" +
	"						Folder to be searched for reproducer results to be added into report\n" +
	"						(Usually, the reproducers checked on slightly defferent kernel states.\n" +
	"						In other words, more INPUT_DIRs to compare to.)\n")
	os.Exit(1)
}


func main() {
	flag.Parse()
	args := flag.Args()

	// Check whther non-defined (unexpected) args are given
	if len(args) > 0 {
		fmt.Println("Unexpected args used starting from: " + args[0])
		usage()
	}

	// Test-read of INPUT DIR?  Do we need that?
	inDir := *flagInDir
	_, err := ioutil.ReadDir(inDir)
	if err != nil {
		log.Fatalf("Failed to read INPUT_DIR '%s': %v", inDir, err)
		usage()
	}

	cmpDirs := []string{*flagCmpDir}  // TODO: Extend the options for unlimited number of CMP Dirs
	generate(*flagInDir, *flagOutFile, *flagCBisctDir, cmpDirs)

	log.Logf(0, "all done.")
}


// Intended to return signle line read stripped out off any unnecessary wrapping
func ReadStrippedln(r *bufio.Reader) (string, error) {
	var (
		isPrfx bool = true
		er error = nil
		line, outLn []byte
	)
	for isPrfx && er == nil {
		line, isPrfx, er = r.ReadLine()
		outLn = append(outLn, line...)
	}
	outLine := string(outLn)
	outLine = strings.TrimSpace(outLine) // trim whitespace
	outLine = strings.Trim(outLine, "\t \n") // trim specific
	return outLine, er
}


func get_crashes(hash_dir string) []string {
	crashes := []string{}

	hashDirList, err := ioutil.ReadDir(hash_dir)
	if err != nil {
		log.Fatal(err)
		// os.Exit(1)  // Any reason to drop everything and give up?
	}

	for _, crash := range hashDirList {
		crash_path := filepath.Join(hash_dir, crash.Name())
		if stat, err := os.Stat(crash_path); err != nil || !stat.IsDir() {
			continue
		}
		desc_file := filepath.Join(crash_path, "description")

		descFileOpen, err := os.Open(desc_file)
		description := ""
		if err != nil {
			log.Fatal(err)
			// os.Exit(1)  // Any reason to drop everything and give up?
		} else {
			rdDescFile := bufio.NewReader(descFileOpen)
			description, err = ReadStrippedln(rdDescFile)
			if err != nil {
				log.Fatal(err)
			}
		}

		crashes = append(crashes, description)
	}

	return crashes
}


func get_kernel_version_and_exit_code(logfile string) (string, string) {
	exit_code := "1"
	kernel_version :=  "TBD!" // None

	// TODO: Convert!

	// // # Reproducer exit code was: 0
	// // # Reproducer exit code was: 124
	// re_exit_code = re.compile(r"Reproducer exit code was: (?P<exit_code>[0-9]+)")
	// // # [    0.000000] Linux version 4.19.59-rt24-eb-corbos-preempt-rt (hogander@GL-434)...
	// re_kernel_version = re.compile(r".*(?P<kernel_version>Linux version.*)")

	// with open(logfile, "r") as stream:
	// 	lines = stream.readlines()

	// for line in lines:
	// 	m = re_kernel_version.search(line)
	// 	if m:
	// 		kernel_version = m.group("kernel_version")
	// 		break

	// for line in reversed(lines):
	// 	m = re_exit_code.search(line)
	// 	if m:
	// 		exit_code = m.group("exit_code")
	// 		break

	return kernel_version, exit_code
}


func checkHandleWriteErr(err error, f *os.File) {
    if err != nil {
		fmt.Println(err)
		log.Fatal(err)
		errMore := f.Close()
		if errMore != nil {
			fmt.Println(errMore)
			log.Fatal(errMore)
		}
		os.Exit(1)
    }
}


func writeStrChkErr(f *os.File, s string) int {
	l, err := f.WriteString(s)
	checkHandleWriteErr(err, f)
	return l
}


func write_report(report map[string][]string, output_file string) {
    fout, err := os.Create(output_file)
    if err != nil {
        fmt.Println(err)
		log.Fatal(err)
		os.Exit(1)  // Really the reason now to drop everything, right?
	}
	l := 0
	// OVT: Refers to not converted part yet (should print out 0?)
	if len(report["kernel_versions"]) > 0 {
		fmt.Println("h5. Found kernel version(s):")
		l += writeStrChkErr(fout, "# Found kernel version(s):\n")
		l += writeStrChkErr(fout, "```\n")
		for _, kernel_version := range report["kernel_versions"] {
			fmt.Println(kernel_version)  // + "\n"
			l += writeStrChkErr(fout, kernel_version + "\n")
		}
		l += writeStrChkErr(fout, "```\n")
	}
	fmt.Println("h5. Summary:")
	l += writeStrChkErr(fout, "# Summary:\n")
	for  _, line := range report["summary"] {
		fmt.Println("    " + line)
		l += writeStrChkErr(fout, "    " + line + "\n")
	}
	fmt.Println("h5. Reproducers:")
	l += writeStrChkErr(fout, "# Reproducers:\n")
	header := report["header"][0]  // OVT: Do we have/use single entry here (or should concat all?)
	fmt.Println("||" + strings.Replace(header, "|", "||", -1) + "||")
	l += writeStrChkErr(fout, header + "\n")
	column_count := len(strings.Split(header, "|"))

	for column_count > 0 {
		column_count -= 1
		l += writeStrChkErr(fout, "-")
		if column_count > 0 {
			l += writeStrChkErr(fout, "|")
		}
	}
	l += writeStrChkErr(fout, "\n")

	for _, status := range statuses {
		for  _, line := range report[status] {
			fmt.Println("|" + line + "|")
			l += writeStrChkErr(fout, line + "\n")
		}
	}
	fmt.Println(l, "bytes written successfully")
    err = fout.Close()
    if err != nil {
        fmt.Println(err)
        log.Fatal(err)
		os.Exit(1)
    }
}


func get_comparison_results(comparison_dirs []string, hash_id string) string {
	comparison_results := ""

	if len(comparison_dirs) > 0 {
		return comparison_results
	}

	hash_dir := ""
	hash_log := ""
	for _, dir := range comparison_dirs {
		hash_dir = filepath.Join(dir, hash_id)
		hash_log = filepath.Join(hash_dir, hash_id + ".log")
		comparison_results = "|"

		if _, err := os.Stat(hash_log); err != nil {
			comparison_results += "Not available"
			continue
		}
		_, exit_code := get_kernel_version_and_exit_code(hash_log)

		crashes := get_crashes(hash_dir)
		if len(crashes) > 0 {
			for _, crash := range crashes {
				comparison_results += crash
				if crash != crashes[len(crashes) - 1] {
					comparison_results += ", "
				}
			}
		} else if exit_code == "124" {
			comparison_results += "TIMED OUT"
		} else if exit_code == "0" {
			comparison_results += "PASSED"
		} else {
			comparison_results += "FAILED"
		}
	}
	return comparison_results
}


// TODO: Convert the code below when func-ty accepted to syzkaller:

// func get_config_bisect_result(results_dir, hash_id) {
// 	config_bisect_dir = os.path.join(results_dir, hash_id)

// 	if not os.path.exists(config_bisect_dir):
// 		return "Not available"

// 	if not os.path.exists(os.path.join(config_bisect_dir, "iteration_1")):
// 		return "Not reproducible"

// 	if not os.path.exists(os.path.join(config_bisect_dir, "iteration_2")):
// 		return "Reproducible with baseline"

// 	bisect_result_file = os.path.join(config_bisect_dir, "bisect_result")
// 	if not os.path.exists(bisect_result_file):
// 		return "FAILED"

// 	with open(bisect_result_file, "r") as stream:
// 		bisect_result = stream.readline()
// 	if bisect_result != "SUCCESS":
// 		return "FAILED"

// 	config_additions_file = os.path.join(config_bisect_dir, "config_additions")
// 	if not os.path.exists(config_additions_file):
// 		return "FAILED"

// 	bisect_results = ""
// 	with open(config_additions_file, "r") as stream:
// 		for addition in stream:
// 			bisect_results = bisect_results.replace("\n", ", ")
// 			bisect_results += addition
// 		bisect_results = bisect_results.strip()

// 	return bisect_results
// }


// It's reported the GoLang doesn't have THIS crucial thing in any of its libs
// Contains tells whether a contains x.
func Contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}


func generate(input_dir, output_file, config_bisect_dir string, comparison_dir []string) {
	report := make(map[string][]string)
	report["kernel_versions"] = []string{}
	report["header"] = append(report["header"], "SHA|Status|Crashes")

	// OVT: TODO: Add this option fucn-ty later (not accepted into the master yet)
	// if config_bisect_dir
	// 	report["header"] += "|Config Bisect"
	if len(comparison_dir) > 1 || len(comparison_dir) > 0 && len(comparison_dir[0]) > 0 {
		for _, dir := range comparison_dir {
			report["header"][0] += "|" + dir
		}
	}
	report["summary"] = []string{}

	for _, status := range statuses {
		report[status] = []string{}
	}
	dirList, err := ioutil.ReadDir(input_dir)
	if err != nil {
		log.Fatal(err)  // TODO: Check if it's needed: didn't we check that actually initially?
		os.Exit(1)
	}
	for _, hash_id := range dirList {
		hash_dir := filepath.Join(input_dir, hash_id.Name())
		if stat, err := os.Stat(hash_dir); err != nil || !stat.IsDir() {
			continue
		}
		crashes := get_crashes(hash_dir)

		hash_log := filepath.Join(hash_dir, hash_id.Name() + ".log")  // os.path.join(hash_dir, hash_id + ".log")
		if _, err := os.Stat(hash_log); err != nil {  // if not os.path.exists(hash_log):
			continue
		}
		// TODO: Check converted!
		kernel_version, exit_code := get_kernel_version_and_exit_code(hash_log)

		if !Contains(report["kernel_versions"], kernel_version) {
			report["kernel_versions"] = append(report["kernel_versions"], kernel_version)
		}
		report_line := hash_id.Name() + "|"

		// OVT: TODO: Add this option fucn-ty later (not accepted into the Syz-master yet)
		// if config_bisect_dir {
		// 	bisect_result := "|N/A"
		// } else {
		 	bisect_result := ""
		// }
		comparison_results := get_comparison_results(comparison_dir, hash_id.Name())

		if len(crashes) > 0 {
			report_line += "CRASHED|"
			for _, crash := range crashes {
				report_line += crash
				if crash != crashes[len(crashes) - 1] {
					report_line += ", "
				}
			}

			// OVT: TODO: Add this option fucn-ty later (not accepted into the Syz-master yet)
			// if config_bisect_dir {
			// 	report_line += "|"
			// 	bisect_result = Report.get_config_bisect_result(config_bisect_dir, hash_id)
			// 	report_line += bisect_result
			// }
			// report_line += comparison_results

			report["crashed"] = append(report["crashed"], report_line)
		} else if exit_code == "124" {
			report_line += "TIMED OUT|" + bisect_result + comparison_results
			report["timed_out"] = append(report["timed_out"], report_line)
		} else if exit_code == "0" {
			report_line += "PASSED|" + bisect_result + comparison_results
			report["passed"] = append(report["passed"], report_line)
		} else {
			report_line += "FAILED|" + bisect_result + comparison_results
			report["failed"] = append(report["failed"], report_line)
		}
	}

	total_repros := 0
	for _, status := range statuses {
		total_repros += len(report[status])
	}
	report["summary"] = append(report["summary"], "Total number of reproducers: " + strconv.Itoa(total_repros))
	report["summary"] = append(report["summary"], "Crashed: " + strconv.Itoa(len(report["crashed"])))
	report["summary"] = append(report["summary"], "Failed: " + strconv.Itoa(len(report["failed"])))
	report["summary"] = append(report["summary"], "Timed out: " + strconv.Itoa(len(report["timed_out"])))
	report["summary"] = append(report["summary"], "passed: " + strconv.Itoa(len(report["passed"])))

	write_report(report, output_file)
}



