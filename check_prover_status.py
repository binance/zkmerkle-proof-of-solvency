import sys
import time
import subprocess
import os

def get_delta():
    p = subprocess.run(["go", "run", "main.go", "-check_prover_status"], stdout=subprocess.PIPE)
    return int(p.stdout.strip().split(b"\n")[1])

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
    print("prev_delta ", prev_delta)
    while True:
        time.sleep(60)
        cur_delta = get_delta()
        print("cur_delta ", cur_delta)
        if prev_delta != cur_delta or prev_delta == 0:
            break

    rerun_retry = 0
    while True:
        time.sleep(2*60)
        cur_delta = get_delta()
        if prev_delta == 0 or cur_delta == 0:
            print("all proofs has been generated")
            break
        if cur_delta == prev_delta:
            print("there is no new proof generate in 2 minutes, it means all prover finished running")
            print("there are ", cur_delta, " proofs need to rerun")
            os.chdir("../prover")
            rerun()
            os.chdir("../dbtool")
            delta = get_delta()
            if delta == 0:
                print("rerun successfully")
                break
            else:
                print("after rerun, there is still ", delta, " proof, will retry...")
                rerun_retry += 1
                if rerun_retry > 3:
                    print("rerun failed too many times, need manually check")
                    exit(0)

        print("current delta is ", cur_delta, " previous delta is ", prev_delta)
        prev_delta = cur_delta

    print("successfully...")