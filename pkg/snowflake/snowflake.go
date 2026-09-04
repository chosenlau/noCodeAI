package snowflake

import (
	"strconv"

	"github.com/sony/sonyflake"
)

var (
	sf *sonyflake.Sonyflake
)

// init 初始化雪花 ID 生成器
func init() {
	sf = sonyflake.NewSonyflake(sonyflake.Settings{
		MachineID: func() (uint16, error) { return 1, nil }, // 机器 ID，分布式环境下应动态获取
	})
}

// GenerateSnowFlakeId 生成雪花 ID（int64）
func GenerateSnowFlakeId() (int64, error) {
	id, err := sf.NextID()
	if err != nil {
		return 0, err
	}
	return int64(id), nil
}

// GenerateSnowFlakeIdString 生成雪花 ID（string）
func GenerateSnowFlakeIdString() (string, error) {
	snowFlakeId, err := GenerateSnowFlakeId()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(int(snowFlakeId)), nil
}
