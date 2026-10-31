package diagnostic

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/locales"
)

const MaxBytes = 8 << 20

type Record struct{ Clock, Level, Source, Message string }
type Finding struct {
	ID, Title, Advice string
	Possible          bool
	Evidence          []string
}
type Report struct {
	Created        time.Time
	Source, System string
	Truncated      bool
	Records        []Record
	Findings       []Finding
}

func Prepare(ctx context.Context, input Report, redactor *Redactor) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	result := Report{Created: input.Created, Source: redactor.Text(input.Source), System: redactor.Text(input.System), Truncated: input.Truncated}
	if result.Created.IsZero() {
		result.Created = time.Now()
	}
	start, size := len(input.Records), 0
	for start > 0 && len(input.Records)-start < 2000 {
		r := input.Records[start-1]
		n := len(r.Clock) + len(r.Level) + len(r.Source) + len(r.Message)
		if n > MaxBytes-size {
			break
		}
		size += n
		start--
	}
	result.Truncated = result.Truncated || start > 0
	result.Records = make([]Record, 0, len(input.Records)-start)
	size = 0
	for _, record := range input.Records[start:] {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		clean := Record{redactor.Text(record.Clock), redactor.Text(record.Level), redactor.Text(record.Source), redactor.Text(record.Message)}
		result.Records = append(result.Records, clean)
		size += len(clean.Clock) + len(clean.Level) + len(clean.Source) + len(clean.Message)
	}
	start = 0
	for size > MaxBytes && start < len(result.Records) {
		r := result.Records[start]
		size -= len(r.Clock) + len(r.Level) + len(r.Source) + len(r.Message)
		result.Records[start] = Record{}
		start++
	}
	if start > 0 {
		result.Truncated = true
		result.Records = result.Records[start:]
	}
	var err error
	result.Findings, err = Analyze(ctx, result.Records)
	return result, err
}

func Analyze(ctx context.Context, records []Record) ([]Finding, error) {
	var findings []Finding
	add := func(id, title, advice, line string, possible bool) {
		index := -1
		for i := range findings {
			if findings[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			findings = append(findings, Finding{ID: id, Title: title, Advice: advice, Possible: possible})
			index = len(findings) - 1
		}
		item := &findings[index]
		line = excerpt(strings.TrimSpace(line), 512)
		for _, old := range item.Evidence {
			if old == line {
				return
			}
		}
		if len(item.Evidence) < 3 {
			item.Evidence = append(item.Evidence, line)
		}
	}
	size := 0
	for i, record := range records {
		if i >= 2000 {
			break
		}
		if len(record.Message) > MaxBytes-size {
			break
		}
		size += len(record.Message)
		for line := range strings.SplitSeq(record.Message, "\n") {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			switch {
			case strings.Contains(line, "Preview features are not enabled"):
				add("java-preview", locales.JavaPreviewFeaturesAreDisabled(), locales.UseTheJavaVersionRequiredByYourModsAndCheckWhetherEnablePreviewIsNeeded(), line, false)
			case strings.Contains(line, "UnsupportedClassVersionError"), strings.Contains(line, "Unsupported major.minor version"):
				advice := locales.CheckTheInstanceJavaVersionAndUseTheRuntimeRequiredByTheGameAndMods()
				if version := requiredJava(line); version > 0 {
					advice = fmt.Sprintf(locales.ThisClassFileRequiresAtLeastJavaAlsoCheckTheGameAndModVersionRequirements(), version)
				}
				add("java-version", locales.IncompatibleJavaVersion(), advice, line, false)
			case strings.Contains(line, "Unrecognized VM option"), strings.Contains(line, "Unrecognized option:"), strings.Contains(line, "Invalid maximum heap size"), strings.Contains(line, "Invalid initial heap size"):
				add("java-arguments", locales.InvalidJavaLaunchArguments(), locales.CheckExtraInstanceJvmArgumentsAndRemoveOptionsUnsupportedByThisJavaVersion(), line, false)
			case strings.Contains(line, "OutOfMemoryError: Java heap space"), strings.Contains(line, "OutOfMemoryError: GC overhead limit exceeded"):
				add("heap-memory", locales.JavaHeapExhausted(), locales.IncreaseTheGameMemoryLimitIfSystemMemoryAllowsOrReduceModsAndResourcePacks(), line, false)
			case strings.Contains(line, "OutOfMemoryError: Metaspace"), strings.Contains(line, "OutOfMemoryError: Compressed class space"):
				add("class-memory", locales.JavaClassMetadataSpaceExhausted(), locales.CheckMetaspaceCompressedClassSpaceSizeLimitsAndModsIncreasingTheGameHeapAloneMayNotHelp(), line, false)
			case strings.Contains(line, "Could not reserve enough space"), strings.Contains(line, "Native memory allocation (malloc) failed"), strings.Contains(line, "There is insufficient memory for the Java Runtime Environment"):
				add("native-memory", locales.JvmCannotAllocateNativeMemory(), locales.CheckAvailableSystemMemoryAddressSpaceAndProcessLimitsReduceTheGameMemoryLimitIfNeeded(), line, false)
			case strings.Contains(line, "OutOfMemoryError: unable to create native thread"):
				add("thread-limit", locales.CouldNotCreateJavaThread(), locales.CheckSystemMemoryAndProcessThreadLimits(), line, false)
			case strings.Contains(line, "Missing mandatory dependencies"), strings.Contains(line, "Missing or unsupported mandatory dependencies"), strings.Contains(line, "which is missing!") && strings.Contains(line, "requires"), strings.Contains(line, "requires version") && strings.Contains(line, "is missing"):
				add("missing-dependency", locales.MissingModDependencies(), locales.InstallTheMissingDependencyModsListedInTheLogAndVerifyLoaderCompatibility(), line, false)
			case strings.Contains(line, "Incompatible mods found!"), strings.Contains(line, "Mod resolution encountered an incompatible mod set"), strings.Contains(line, "Found incompatible mods"):
				add("mod-compatibility", locales.LoaderReportedIncompatibleMods(), locales.CheckTheGameLoaderAndDependencyVersionsRequiredByTheMods(), line, false)
			case strings.Contains(line, "NoClassDefFoundError:"), strings.Contains(line, "ClassNotFoundException:"):
				add("missing-class", locales.ClassOrDependencyCouldNotBeLoaded(), locales.ARequiredModMayBeMissingOrVersionsMayConflictCheckTheLoaderErrorsAbove(), line, true)
			case strings.Contains(line, "Could not find or load main class"), strings.Contains(line, "Unable to access jarfile"):
				add("launch-files", locales.MainClassOrJarCouldNotBeLoaded(), locales.CheckJavaAndCustomArgumentsThenVerifyAndRepairTheInstance(), line, false)
			case strings.Contains(line, "java.util.zip.ZipException:") && (strings.Contains(line, "zip END header not found") || strings.Contains(line, "invalid LOC header") || strings.Contains(line, "invalid CEN header")):
				add("archive-damage", locales.DamagedJarOrZipStructure(), locales.ReDownloadTheCorruptedFileNamedInTheLogOrRunVerifyAndRepairForGameFiles(), line, false)
			case strings.Contains(line, "A fatal error has been detected by the Java Runtime Environment"), strings.Contains(line, "Problematic frame:"):
				add("native-crash", locales.JvmNativeCrash(), locales.CheckTheProblematicFrameInHsErrPidThenReviewJavaNativeLibraryAndDriverVersions(), line, true)
			}
		}
	}
	return findings, ctx.Err()
}

func requiredJava(line string) int {
	for _, marker := range []string{"class file version ", "Unsupported major.minor version "} {
		_, tail, ok := strings.Cut(line, marker)
		if !ok {
			continue
		}
		value := 0
		for i := 0; i < len(tail) && i < 3 && tail[i] >= '0' && tail[i] <= '9'; i++ {
			value = value*10 + int(tail[i]-'0')
		}
		if value >= 49 && value <= 100 {
			return value - 44
		}
	}
	return 0
}

func excerpt(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + "…"
}
