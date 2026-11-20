def parse_positive_timeout(raw):
    try:
        timeout = int(raw)
    except ValueError as exc:
        raise ValueError("超时配置必须是整数") from exc

    if timeout <= 0:
        raise ValueError("超时秒数必须大于 0")

    return timeout

def load_timeouts(raw_values):
   success = []
   fail = []
   for value in raw_values:
       try:
           timeout = parse_positive_timeout(value)
           success.append(timeout)
       except ValueError as exc:
           fail.append((value, exc.__str__()))
   return (success, fail)

def main():
    print(load_timeouts(["30", "abc", "-5", "5"]))

if __name__ == "__main__":
    main()