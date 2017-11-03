package visor

import "github.com/skycoin/skycoin/src/coin"

const (
	// Maximum supply of skycoins
	MaxCoinSupply uint64 = 3e8 // 100,000,000 million

	// Number of distribution addresses
	DistributionAddressesTotal uint64 = 100

	DistributionAddressInitialBalance uint64 = MaxCoinSupply / DistributionAddressesTotal

	// Initial number of unlocked addresses
	InitialUnlockedCount uint64 = 100

	// Number of addresses to unlock per unlock time interval
	UnlockAddressRate uint64 = 5

	// Unlock time interval, measured in seconds
	// Once the InitialUnlockedCount is exhausted,
	// UnlockAddressRate addresses will be unlocked per UnlockTimeInterval
	UnlockTimeInterval uint64 = 60 * 60 * 24 * 365 // 1 year
)

func init() {
	if MaxCoinSupply%DistributionAddressesTotal != 0 {
		panic("MaxCoinSupply should be perfectly divisible by DistributionAddressesTotal")
	}
}

// Returns a copy of the hardcoded distribution addresses array.
// Each address has 1,000,000 coins. There are 100 addresses.
func GetDistributionAddresses() []string {
	addrs := make([]string, len(distributionAddresses))
	for i := range distributionAddresses {
		addrs[i] = distributionAddresses[i]
	}
	return addrs
}

// Returns distribution addresses that are unlocked, i.e. they have spendable outputs
func GetUnlockedDistributionAddresses() []string {
	// The first InitialUnlockedCount (30) addresses are unlocked by default.
	// Subsequent addresses will be unlocked at a rate of UnlockAddressRate (5) per year,
	// after the InitialUnlockedCount (30) addresses have no remaining balance.
	// The unlock timer will be enabled manually once the
	// InitialUnlockedCount (30) addresses are distributed.

	// NOTE: To have automatic unlocking, transaction verification would have
	// to be handled in visor rather than in coin.Transactions.Visor(), because
	// the coin package is agnostic to the state of the blockchain and cannot reference it.
	// Instead of automatic unlocking, we can hardcode the timestamp at which the first 30%
	// is distributed, then compute the unlocked addresses easily here.

	addrs := make([]string, InitialUnlockedCount)
	for i := range distributionAddresses[:InitialUnlockedCount] {
		addrs[i] = distributionAddresses[i]
	}
	return addrs
}

// Returns distribution addresses that are locked, i.e. they have unspendable outputs
func GetLockedDistributionAddresses() []string {
	// TODO -- once we reach 30% distribution, we can hardcode the
	// initial timestamp for releasing more coins
	addrs := make([]string, DistributionAddressesTotal-InitialUnlockedCount)
	for i := range distributionAddresses[InitialUnlockedCount:] {
		addrs[i] = distributionAddresses[InitialUnlockedCount+uint64(i)]
	}
	return addrs
}

// Returns true if the transaction spends locked outputs
func TransactionIsLocked(inUxs coin.UxArray) bool {
	lockedAddrs := GetLockedDistributionAddresses()
	lockedAddrsMap := make(map[string]struct{})
	for _, a := range lockedAddrs {
		lockedAddrsMap[a] = struct{}{}
	}

	for _, o := range inUxs {
		uxAddr := o.Body.Address.String()
		if _, ok := lockedAddrsMap[uxAddr]; ok {
			return true
		}
	}

	return false
}

var distributionAddresses = [DistributionAddressesTotal]string{
	"Z1k6qej1yPoNAZmRVCGQW8t5zyW2b8EYSa",
	"2BWPusJggEF8zHTdQyy62oTAre8m326kNTG",
	"2abzMYGi6HFP8F8ZyUcdWY1fXTvgncLxxUg",
	"2cP8ed8C7ugK2BozeVnVhVfFQ1HL1tixsPq",
	"yQ4FP8iRjN7fwLwi3RrJZaNJf63LNoijKo",
	"2epqomKZVWGxaDpaZkwu9QHM9RaFsAHgziQ",
	"SLKXabjERNatNZfrWQDzVvxJt2SSYMx7DN",
	"xYS16WXoa7qoN6CCibmPtW938F7JiNYJmo",
	"9u1jDCeK7WSVs1wuoQYK3GD3PqUGtTNLUz",
	"2FbNyvNqMzEvrRrv4kcsp4GxLZQPWHzUwru",
	"2Z5dMNPe9Dd5FPTWWwTsRfREwBf53wftz3G",
	"2ZpN1NMvoM3brt1LiQbxSUT1nK14tiTKmjs",
	"qGFwwYHJtYSH26Bg3E33Zi7EQXMj45ECU7",
	"2Y6gK4992vHPJJ1cUQa7GmkHTophhqkfqNp",
	"2Ur7H3NJ6uivPx4W1uuDhfeu92E7tXFgbbk",
	"yX78JsBww6qRM9Cy6r1cMcM1cqqfvLB81G",
	"2Wu7pUcSQVXBMF6iHT5GK1JRHToYc5VwEMU",
	"sX8V7KE5XDgtFsxUTF8nJoXCAS4WG6qmdL",
	"BMzo8uGGtTswKCLiUGQYQEC3TSgFUYxGUV",
	"WYUGd1PJtFkjAtzFULcYcXwzM4RwD4Bohg",
	"8D3QqVJ1hLhmwK9dH6CjK8j8PmzBZdFXKi",
	"zZLbkWDxiZPvziLXiT8Bgir1bP94dGkSv8",
	"cDAzgCAxKphwdhWB5ziSxrBzS8J3FxKrVX",
	"W64hnPJ8h48CErFoAJDunC7FDTXCiDBYaU",
	"rkTX8iDP4FbZHkomBze6UkHDmhvw54vrci",
	"2i4iBDtiagR3jiegnmbiVZ1tkTp6vwVFjXF",
	"wFCxPvzVBce1APxbFGgpPFnTN5cQUnLqEp",
	"KLW1sJUTuGQcQATaMrkMeEzMhKqFE9SzrM",
	"2zxjynmyjHwyA9CmtEpnKBUc2t3k7L7YxT",
	"ZpcAH9SdsUfgPgJUt7kY5FdvSkxMAy7AmW",
	"XV7YcAQujembG8pWksdjqraWfoGdpxUcfy",
	"Exskppefm5J35ejwS7HQTRUtCKNacwbENc",
	"Y7ngaBfePwGy4vpNgyxXcw6EdHkuNkLdh",
	"2VW4y4MjEceMtkHkoPPC5DCVATZJJ4wFd61",
	"2jF6FVDs3trWCg2AjdTGBbJbqBL1zo83aXV",
	"eAdXVjvfwATdMsQNxWCwn9fmrjj4yV1gYk",
	"JnuJTVDw2SBh559G4TKVdDSMZXgizBgAWX",
	"2XjofbRTnCU5F5PFMErLmivfveCJj1PyyKX",
	"jY7D2siBSdtCnW2qZQxtAHNMiVtzp72dpy",
	"Pqho6tzNGn1oCjx67oJ64FjZsodcbCfZA8",
	"2LxCTzJFfNCxDvkhq8cgfq4P6T2bK5qVxKh",
	"Wf2ZE5NswFjSiMc4beFG3kG5jppMU77NA",
	"7ThrKqp68NLgSCAvFq75CghNDCVzPEV8pb",
	"2FXWH1yNkrpSm99uggshpAxyRxu4BDsLp43",
	"2VcS5bRbATZRfyfGqxtdcVsCMnFcU5YRC5w",
	"cAyxv5CRpUWqNZesjU4dsw9zsgp3wo1mzC",
	"uxRrW927Thp9kBUqCCF5uGizQNYN9GJZBb",
	"usSvw63y1HrHxYpksjBhcnrFSDQao619KN",
	"urVo1mKQa4wZR79ViFeDfsBA1hmNRz4hgA",
	"21wFQjVV8HvWtKcyA9gzb8rQeFLYUFRE5S1",
	"2C8mNpRfEsAfjMYqc8DwqdzYz1XsWdX8Lpg",
	"2SVd9pGm59rafUAc3i5fWUwiEvc9QDz7Cqd",
	"ZRvUwGFJ58y5H9rwUEqJNdPEr4DiAjRgHm",
	"H7a1oj4KvWKkGntnHh2uZoGyrvM3pJgGze",
	"4YffwkMK5Fmxc5KcXKaQs1QJU9yLHijrrx",
	"2TkyDHnoGJcLbdsNYFUwccm113GF8MMCETj",
	"2CFTZWhHDEXwX1N1NRwNZ9bEErzw82SKNyx",
	"Nxrs7pRYWGbxEByhZk14qJoJYbUuPgF1LD",
	"2GbczQuUZe3CDt6fQEkmmYxruTZ2JW79uAs",
	"2UoAuTGkA8xq6bh9Hs6KXNTLkD459THrxiy",
	"q6w4yZbzLoNWmfq5MopSwPkzPJqrroWq8v",
	"Ee6bq2Gmxp8m5aCX4jvG1yBPawLvTttGNX",
	"2MSTLjxJm2oPLzQCYrbm1zkFR5xEnQeFi3h",
	"27WRJ87b3DxEvmh8axkgEews8DYycjN91xm",
	"29b2GCEQrM7vF7pngHKtvVrTXCL1DJU3s9Y",
	"2NrKPH7UH3jhhEW7JVsExLpTtJQ6kYfcmw1",
	"SWPF7b9d6KPn3XXiq6tdkoLcWTiPpTD7Ke",
	"2eQf3cgtUVSpNScJjwNgoEyHuVXjao6GXw3",
	"2Ltm64ZjNcaP85jNUgucpKNHZDcrnFmiyEk",
	"WuRJen3KavvZxyKVaPHcUvJW6CtWT33iRp",
	"pf4g5BXa3Scq5yRiX7redAP6c3up6BrysT",
	"aGx9x1qMqNyxDVwD5PY7hQ1RaYCuTX6GX5",
	"2hU1xk3Zy4NrfAKuB28ZtEbQNPum6pjjZ2b",
	"21p72H7FJ1g3jMM178ozj1BjzsAnkKskYBm",
	"5f3Rh5dPHGoV1z1kbWNApJVreQUAKaJiXz",
	"wZJjmL184D1ZyfmHmfrZX8UKH6RyEYFMJ8",
	"BxvJsGCV1n2G7PZEEE3ZQuUeSNAfszEcWB",
	"2YUhuUVPJERRCo5d8KMZFRXE5XZdmHus547",
	"tUq54Hdd4Pyp5vEXN7zC5fNk1UjyomgTae",
	"A2vjGTqvXyTaopoUhiNiNyJKoRrXPRobSm",
	"npfUWxQg42wWNXwYhUPAS76cDgVtm44H4n",
	"gYuosHMTDoYTEgmUkYoDppZy1PjJRW5eyB",
	"J45zxYi9UUUvCK5Pv51gHPH1a33FuPL2LJ",
	"2LDfyyL9EL9gSe3aC5rM8EpMh4zn3BJWfk",
	"2cH1VBMKSWW6WftBs7CGwLrWESTkxhc2xMX",
	"tmEmVBTDxTpbGNwMdBJLTWrLn4VnpFphmZ",
	"DjemT2f8iAwk7MnxMcpjy75urf1Vz5rgsV",
	"2RRa36ELR8jDm8m5Mf6DS3tskMw1PknCxAz",
	"cEKEgZyjjMR2mmnfFHgmFjAvVwmzeowiYg",
	"A2ohw5ZgMHdsJpskEgd4pQny3DtH54Sfqi",
	"2bFQnRXmFr3coS7v4tJh6AC4mfUJTTRVQME",
	"PLsUxXTJJWKuZVnMxP3FYbnMk1DBxXyBoJ",
	"n5fMHy77JrfQawvZ4SKfsiVkfvuN1YnCyQ",
	"2f2Uu4BbbcWMX4qe1uFm8UgPsZkHsCB7t5U",
	"W1SQKqzBDxVyVkkjTx1aEFFC1vDaf6cujg",
	"Yf9D1EHKL5tjAiNjsVTcon5aARz7orb4mo",
	"28eVNsyvCYx85qgwF9FYJLseKyoknDVig21",
	"pYzWzz1oxFbdtg2UrXmebNvhmzhGbBt9Q",
	"afAj8nZ92jBMnMNcDKyHSkAyBEHf65tW6n",
	"2mB9UQbsiB4jFooBLYgnBk3hB8GnEEYVadz",
}
