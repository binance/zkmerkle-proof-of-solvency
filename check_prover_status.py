import sys
import time
import subprocess
import os

def get_delta():
    p = subprocess.run(["go", "run", "main.go", "-check_prover_status"], stdout=subprocess.PIPE)
    return int(p.stdout.strip().split(b"\n")[-1])

def rerun():
    process = subprocess.Popen(["go", "run", "main.go", "-rerun"], stdout=subprocess.PIPE)
    while True:
        output = process.stdout.readline()
        if output == b'' and process.poll() is not None:
            break
        if output:
            print(output.strip())
    rc = process.poll()
    return rc

if __name__ == "__main__":
    os.chdir("src/dbtool")
    prev_delta = get_delta()
    print("prev_delta ", prev_delta, flush=True)
    while True:
        time.sleep(60)
        cur_delta = get_delta()
        print("cur_delta ", cur_delta, flush=True)
        if prev_delta != cur_delta or prev_delta == 0:
            break

    rerun_retry = 0
    while True:
        time.sleep(8*60)
        cur_delta = get_delta()
        if prev_delta == 0 or cur_delta == 0:
            print("all proofs has been generated", flush=True)
            break
        if cur_delta == prev_delta:
            print("there is no new proof generate in 8 minutes, it means all prover finished running", flush=True)
            print("there are ", cur_delta, " proofs need to rerun", flush=True)
            os.chdir("../prover")
            rerun()
            os.chdir("../dbtool")
            delta = get_delta()
            if delta == 0:
                print("rerun successfully")
                break
            else:
                print("after rerun, there is still ", delta, " proof, will retry...", flush=True)
                rerun_retry += 1
                if rerun_retry > 3:
                    print("rerun failed too many times, need manually check", flush=True)
                    exit(0)

        print("current delta is ", cur_delta, " previous delta is ", prev_delta, flush=True)
        prev_delta = cur_delta

    print("successfully...", flush=True)