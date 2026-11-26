package pilot
import "os"
func benchmarkEnabled()bool{return os.Getenv("COTE_FIRST_EXAM_BENCH")=="1"}
