"""H2-E: detached, private operational receipt; never a causal authority."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

SCHEMA = "csfrv1-10h.operational-evidence.v1"
MARKERS = frozenset({
    "FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED",
    "H2_FIVE_CANONICAL_LEDGERS=VERIFIED_PRIVATE",
    "H2_SIGNED_OWNER_CHECKPOINTS=VERIFIED_PRIVATE",
    "H2_GENESIS_BOUND_SIGNED_INGRESS=VERIFIED_PRIVATE",
    "H2_RAW_PYTHON_A1_CUTS=SEALED_AND_VERIFIED_PRIVATE",
    "H2_SAME_SIGNED_A1_INDEPENDENT_GO_D3=BYTE_IDENTICAL_AUTHORITIES",
    "CSFRV1_10_V1_DOD_ACCEPTED=NO",
    "FIRST_REAL_RUN_PRODUCTION_GO=NO",
    "LOCAL_REFERENCE_ARCHIVE_VERIFIED=YES",
    "PRODUCTION_APPROVAL=NO",
})
VERIFY_REQUIRED = MARKERS - {
    "FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED",
    "CSFRV1_10_V1_DOD_ACCEPTED=NO",
    "FIRST_REAL_RUN_PRODUCTION_GO=NO",
}
STATES = dict(enumerate(("LOCAL_REFERENCE_OK", "UNCLASSIFIED_FAILURE", "INVALID_ARGUMENTS")))
STATES.update(dict(zip(range(10, 21), (
    "WAIT_FOR_SOURCE_CLOSURE", "REJECTED_SOURCE", "ATTEMPT_IN_FLIGHT",
    "HALTED_ON_ABORT", "RETRY_AUTHORIZED", "CRASH_BETWEEN_FENCE_AND_COMMIT",
    "PENDING_EPISTEMIC_BARRIER", "INCOMPATIBLE_VERSION_OR_POLICY",
    "CORRUPT_OR_UNVERIFIED", "NO_CLASSIFIED_STOP", "SOURCE_CHANGED_DURING_DIAGNOSIS",
))))
STATUS_MARKERS = frozenset(
    [f"LOCAL_REFERENCE_STATE={state}" for state in STATES.values()]
    + [f"LOCAL_REFERENCE_EXIT_CODE={code}" for code in STATES]
    + [f"LOCAL_REFERENCE_NO_GO: {state}" for state in STATES.values()]
)
ALLOWED = MARKERS | STATUS_MARKERS
SUCCESS_MARKERS = {"LOCAL_REFERENCE_STATE=LOCAL_REFERENCE_OK", "LOCAL_REFERENCE_EXIT_CODE=0"}
COMMANDS = {"out": ["csf-reference-run-local", "--out", "<archive>"],
            "verify": ["csf-reference-run-local", "--verify", "<archive>"]}
PIN_LABELS = {"LOCAL_REFERENCE_SOURCE_SHA": ("source_sha", 40),
              "LOCAL_REFERENCE_RUN_ID": ("run_id", 64),
              "LOCAL_REFERENCE_REGISTRATION_SHA256": ("registration_sha256", 64)}


class EvidenceError(ValueError):
    """Only fixed source-code validation reasons; never raw exceptions."""


def require(condition, message):
    if not condition:
        raise EvidenceError(message)


def now():
    return datetime.now(timezone.utc).isoformat(timespec="microseconds")


def timestamp(value):
    require(isinstance(value, str), "invalid operational timestamp")
    parsed = datetime.fromisoformat(value)
    require(parsed.utcoffset() == timezone.utc.utcoffset(None), "timestamp is not UTC")
    return parsed


def canonical(value):
    return (json.dumps(value, sort_keys=True, indent=2, ensure_ascii=True) + "\n").encode()


def hex_digest(value, length=64):
    return isinstance(value, str) and re.fullmatch(f"[0-9a-f]{{{length}}}", value) is not None


def digest(path, private=False):
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1, "unsafe file identity")
    if private:
        require(info.st_mode & 0o077 == 0, "exposed evidence file")
        require(info.st_size <= 128 * 1024 * 1024, "oversized evidence file")
    with path.open("rb") as stream:
        actual = os.fstat(stream.fileno())
        require((actual.st_dev, actual.st_ino) == (info.st_dev, info.st_ino), "file replaced")
        value = hashlib.file_digest(stream, "sha256").hexdigest()
        after = os.fstat(stream.fileno())
    require((after.st_size, after.st_mtime_ns, after.st_ctime_ns, after.st_mode, after.st_nlink) ==
            (info.st_size, info.st_mtime_ns, info.st_ctime_ns, info.st_mode, info.st_nlink),
            "file changed during read")
    return value


def absolute(path):
    value = Path(os.path.abspath(path))
    require(value.resolve() == value, "symlink path refused")
    return value


def private_dir(path):
    info = path.lstat()
    require(stat.S_ISDIR(info.st_mode) and info.st_mode & 0o077 == 0, "unsafe private directory")


def disjoint(a, b):
    return a != b and a not in b.parents and b not in a.parents


def checkout():
    root = Path(subprocess.check_output(["git", "rev-parse", "--show-toplevel"],
                                        text=True, stderr=subprocess.PIPE).strip())
    sha = subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"],
                                  text=True, stderr=subprocess.PIPE).strip()
    dirty = subprocess.check_output(["git", "-C", str(root), "status", "--porcelain",
                                     "--untracked-files=all"], stderr=subprocess.PIPE)
    require(hex_digest(sha, 40) and not dirty, "exact clean source checkout required")
    return root, sha


def binary_pin(binary, sha):
    before = digest(binary)
    build = subprocess.check_output(["go", "version", "-m", str(binary)],
                                    text=True, stderr=subprocess.PIPE)
    rows = [line.strip().split("\t") for line in build.splitlines()[1:]]
    require(["path", "github.com/carloshgalves/cote-ai-simulation/cmd/csf-reference-run-local"] in rows
            and ["build", f"vcs.revision={sha}"] in rows
            and ["build", "vcs.modified=false"] in rows, "incompatible executable revision")
    require(digest(binary) == before, "executable changed during pinning")
    return before


def capture(binary, archive, operation):
    start = now()
    result = subprocess.run([str(binary), COMMANDS[operation][1], str(archive)],
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
    end = now()
    # Exact whole-line allowlist: no paths, arbitrary errors, actor
    # content, environment or private DB bytes. Public pre-causal pins
    # are parsed separately and only when their closed format is valid.
    lines = result.stdout.decode("utf-8", errors="replace").splitlines()
    log = [line for line in lines if line in ALLOWED]
    identity = {}
    for label, (field, length) in PIN_LABELS.items():
        values = [line[len(label) + 1:] for line in lines if line.startswith(label + "=")]
        if len(values) == 1 and hex_digest(values[0], length):
            identity[field] = values[0]
    return {"operation": operation, "command": COMMANDS[operation], "started_at_utc": start,
            "finished_at_utc": end, "process_returncode": result.returncode,
            "redacted_log": log, "public_identity": identity}


def success(step):
    expected = MARKERS if step["operation"] == "out" else VERIFY_REQUIRED
    observed = set(step["redacted_log"])
    return (step["process_returncode"] == 0 and expected | SUCCESS_MARKERS <= observed
            and not any(line.startswith("LOCAL_REFERENCE_NO_GO:") for line in observed)
            and not any(line.startswith(("LOCAL_REFERENCE_STATE=", "LOCAL_REFERENCE_EXIT_CODE="))
                        and line not in SUCCESS_MARKERS for line in observed))


def check_step(step, operation):
    require(isinstance(step, dict) and set(step) == {
        "operation", "command", "started_at_utc", "finished_at_utc",
        "process_returncode", "redacted_log", "public_identity"}, "invalid step schema")
    require(step["operation"] == operation and step["command"] == COMMANDS[operation],
            "wrong command or order")
    require(type(step["process_returncode"]) is int and isinstance(step["redacted_log"], list)
            and all(isinstance(line, str) and line in ALLOWED for line in step["redacted_log"]),
            "invalid exit status or unredacted log")
    require(timestamp(step["started_at_utc"]) <= timestamp(step["finished_at_utc"]),
            "operational clock moved backwards")
    identity = step["public_identity"]
    require(isinstance(identity, dict), "invalid public pin format")
    if operation == "out":
        require(set(identity) == {field for field, _ in PIN_LABELS.values()}
                and all(hex_digest(identity[field], length) for field, length in PIN_LABELS.values()),
                "missing or ambiguous pre-causal identity")
    else:
        require(not identity and not (set(step["redacted_log"]) & (MARKERS - VERIFY_REQUIRED)),
                "verify emitted out-only proof")
    require(success(step), "process did not complete with all required markers")


def snapshot(archive):
    private_dir(archive)
    files = {}
    def walk_error(error):
        raise error

    for root, dirs, names in os.walk(archive, followlinks=False, onerror=walk_error):
        for name in dirs:
            path = Path(root) / name
            private_dir(path)
            files[path.relative_to(archive).as_posix()] = "PRIVATE_DIRECTORY"
        for name in names:
            path = Path(root) / name
            files[path.relative_to(archive).as_posix()] = digest(path, private=True)
    require("checksums.json" in files and "run-manifest.json" in files, "incomplete archive")
    return files


def archive_pins(archive, files, sha):
    manifest_path = archive / "run-manifest.json"
    require(manifest_path.stat().st_size <= 64 * 1024, "oversized run manifest")
    raw = manifest_path.read_bytes()
    require(hashlib.sha256(raw).hexdigest() == files["run-manifest.json"], "manifest changed")
    manifest = json.loads(raw)
    require(manifest["source_sha"] == sha and hex_digest(manifest["run_id"])
            and hex_digest(manifest["registration_sha256"]) and hex_digest(manifest["genesis_sha256"])
            and manifest["disposition"] == "EXPERIMENTAL_LOCAL_ONLY_NO_PRODUCTION_GO"
            and manifest["evidence_profile"] == "H2_SAME_A1_REPLAY_V1", "wrong archive identity")
    require(files.get("registration.json") == manifest["registration_sha256"], "registration changed")
    return {"index_sha256": files["checksums.json"], "manifest_sha256": files["run-manifest.json"],
            "run_id": manifest["run_id"], "genesis_sha256": manifest["genesis_sha256"],
            "registration_sha256": manifest["registration_sha256"],
            "manifest_started_at_utc": manifest["started_at_utc"]}


def validate(receipt, sha, binary_sha, pins):
    require(isinstance(receipt, dict) and set(receipt) == {
        "schema", "source_sha", "binary_sha256", "disposition", "production_approval",
        "archive", "steps"}, "invalid receipt schema")
    require(receipt["schema"] == SCHEMA and receipt["source_sha"] == sha
            and receipt["binary_sha256"] == binary_sha and receipt["archive"] == pins
            and receipt["disposition"] == "LOCAL_OBSERVED_NOT_EXTERNALLY_ATTESTED"
            and receipt["production_approval"] is False, "receipt pins or disposition differ")
    steps = receipt["steps"]
    require(isinstance(steps, list) and len(steps) == 2, "wrong number of commands")
    check_step(steps[0], "out")
    check_step(steps[1], "verify")
    require(steps[0]["public_identity"] == {
        "source_sha": sha, "run_id": pins["run_id"],
        "registration_sha256": pins["registration_sha256"]}, "pre-causal pins differ from archive")
    require(timestamp(steps[0]["started_at_utc"]) <= timestamp(pins["manifest_started_at_utc"])
            <= timestamp(steps[0]["finished_at_utc"]) <= timestamp(steps[1]["started_at_utc"]),
            "manifest or command chronology differs")


def write_receipt(evidence, receipt):
    raw = canonical(receipt)
    require(len(raw) <= 64 * 1024, "oversized receipt")
    fd = os.open(evidence / "operational-receipt.json",
                 os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())
    directory_fd = os.open(evidence, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(directory_fd)
    finally:
        os.close(directory_fd)
    return hashlib.sha256(raw).hexdigest()


def record(binary, archive, evidence, root, sha, binary_sha):
    require(not archive.exists() and not evidence.exists(), "fresh sibling directories required")
    evidence.mkdir(mode=0o700)
    receipt = {"schema": SCHEMA, "source_sha": sha, "binary_sha256": binary_sha,
               "disposition": "INCOMPLETE_NO_GO", "production_approval": False,
               "archive": None, "steps": []}
    try:
        step = capture(binary, archive, "out")
        receipt["steps"].append(step)
        check_step(step, "out")
        before = snapshot(archive)
        pins = archive_pins(archive, before, sha)
        require(digest(binary) == binary_sha, "executable changed before verification")
        step = capture(binary, archive, "verify")
        receipt["steps"].append(step)
        check_step(step, "verify")
        require(snapshot(archive) == before, "cold verifier changed archive bytes")
        require(checkout() == (root, sha) and digest(binary) == binary_sha,
                "source or executable changed during record")
        complete = {**receipt, "archive": pins, "disposition": "LOCAL_OBSERVED_NOT_EXTERNALLY_ATTESTED"}
        validate(complete, sha, binary_sha, pins)
        receipt = complete
    finally:
        pin = write_receipt(evidence, receipt)
        # Even failure gets a private receipt, but never a completion marker.
        for step in receipt["steps"]:
            for line in step["redacted_log"]:
                print(line)
        print("H2_E_RECEIPT_SHA256=" + pin)
        print("H2_E_DISPOSITION=" + receipt["disposition"])
    print("H2_E_REAL_COMMANDS=OBSERVED_OUT_AND_VERIFY_EXIT_0")
    print("H2_E_PRODUCTION_APPROVAL=NO")


def verify(binary, archive, evidence, root, sha, binary_sha, receipt_sha):
    private_dir(evidence)
    path = evidence / "operational-receipt.json"
    require({item.name for item in evidence.iterdir()} == {path.name}, "unexpected receipt files")
    require(hex_digest(receipt_sha) and digest(path, private=True) == receipt_sha,
            "receipt differs from external pin")
    require(path.stat().st_size <= 64 * 1024, "oversized receipt")
    raw = path.read_bytes()
    require(hashlib.sha256(raw).hexdigest() == receipt_sha, "receipt changed")
    receipt = json.loads(raw)
    require(raw == canonical(receipt), "noncanonical or duplicate receipt fields")
    before = snapshot(archive)
    pins = archive_pins(archive, before, sha)
    validate(receipt, sha, binary_sha, pins)
    step = capture(binary, archive, "verify")
    check_step(step, "verify")
    require(snapshot(archive) == before and digest(path, private=True) == receipt_sha
            and {item.name for item in evidence.iterdir()} == {path.name}
            and checkout() == (root, sha) and digest(binary) == binary_sha,
            "archive, receipt, source or executable changed during verification")
    print("H2_E_COLD_RECEIPT=VERIFIED_PRIVATE_NO_WRITE")
    print("H2_E_PRODUCTION_APPROVAL=NO")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=["record", "verify"])
    parser.add_argument("--binary", required=True)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--evidence", required=True)
    parser.add_argument("--receipt-sha256")
    args = parser.parse_args()
    os.umask(0o077)
    try:
        root, sha = checkout()
        binary, archive, evidence = map(absolute, (args.binary, args.archive, args.evidence))
        require(disjoint(archive, evidence) and disjoint(root, archive) and disjoint(root, evidence)
                and disjoint(binary, archive) and disjoint(binary, evidence) and disjoint(root, binary),
                "source, executable, archive and receipt paths must be disjoint")
        binary_sha = binary_pin(binary, sha)
        if args.operation == "record":
            require(args.receipt_sha256 is None, "receipt pin is verify-only")
            record(binary, archive, evidence, root, sha, binary_sha)
        else:
            verify(binary, archive, evidence, root, sha, binary_sha, args.receipt_sha256)
    except EvidenceError as error:
        # Only require() creates this exception with a fixed source literal.
        print("H2_E_FAILURE_REASON=" + str(error), file=sys.stderr)
        print("H2_E_NO_GO=OPERATIONAL_EVIDENCE_UNVERIFIED", file=sys.stderr)
        return 1
    except (OSError, ValueError, TypeError, KeyError, subprocess.SubprocessError):
        # Never emit raw OS/JSON/process exception text or paths.
        print("H2_E_NO_GO=OPERATIONAL_EVIDENCE_UNVERIFIED", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
