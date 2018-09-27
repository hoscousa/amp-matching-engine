package services

import (
	"github.com/Proofsuite/amp-matching-engine/interfaces"
	"github.com/Proofsuite/amp-matching-engine/types"
	"github.com/Proofsuite/amp-matching-engine/utils"
	"github.com/Proofsuite/amp-matching-engine/ws"

	"github.com/ethereum/go-ethereum/common"
)

// TradeService struct with daos required, responsible for communicating with daos.
// TradeService functions are responsible for interacting with daos and implements business logics.
type TradeService struct {
	tradeDao interfaces.TradeDao
}

// NewTradeService returns a new instance of TradeService
func NewTradeService(TradeDao interfaces.TradeDao) *TradeService {
	return &TradeService{TradeDao}
}

// Subscribe
func (s *TradeService) Subscribe(conn *ws.Conn, bt, qt common.Address) {
	socket := ws.GetTradeSocket()

	trades, err := s.GetAllTradesByPairAddress(bt, qt)
	if err != nil {
		socket.SendErrorMessage(conn, err.Error())
		return
	}

	id := utils.GetTradeChannelID(bt, qt)
	err = socket.Subscribe(id, conn)
	if err != nil {
		message := map[string]string{
			"Code":    "UNABLE_TO_REGISTER",
			"Message": "UNABLE_TO_REGISTER " + err.Error(),
		}

		socket.SendErrorMessage(conn, message)
		return
	}

	ws.RegisterConnectionUnsubscribeHandler(conn, socket.UnsubscribeHandler(id))
	socket.SendInitMessage(conn, trades)
}

// Unsubscribe
func (s *TradeService) Unsubscribe(conn *ws.Conn, bt, qt common.Address) {
	socket := ws.GetTradeSocket()

	id := utils.GetTradeChannelID(bt, qt)
	socket.Unsubscribe(id, conn)
}

// GetByPairName fetches all the trades corresponding to a pair using pair's name
func (s *TradeService) GetByPairName(pairName string) ([]*types.Trade, error) {
	return s.tradeDao.GetByPairName(pairName)
}

func (s *TradeService) GetRecentTradesByPairAddress(baseToken, quoteToken common.Address) ([]*types.Trade, error) {
	return s.tradeDao.GetRecentTradesByPairAddress(baseToken, quoteToken)
}

// GetByPairAddress fetches all the trades corresponding to a pair using pair's token address
func (s *TradeService) GetAllTradesByPairAddress(baseToken, quoteToken common.Address) ([]*types.Trade, error) {
	return s.tradeDao.GetAllTradesByPairAddress(baseToken, quoteToken)
}

func (s *TradeService) GetNTradesByPairAddress(baseToken, quoteToken common.Address, n int) ([]*types.Trade, error) {
	return s.tradeDao.GetNTradesByPairAddress(baseToken, quoteToken, n)
}

// GetByUserAddress fetches all the trades corresponding to a user address
func (s *TradeService) GetByUserAddress(addr common.Address) ([]*types.Trade, error) {
	return s.tradeDao.GetByUserAddress(addr)
}

// GetByHash fetches all trades corresponding to a trade hash
func (s *TradeService) GetByHash(h common.Hash) (*types.Trade, error) {
	return s.tradeDao.GetByHash(h)
}

// GetByOrderHash fetches all trades corresponding to an order hash
func (s *TradeService) GetByOrderHash(h common.Hash) ([]*types.Trade, error) {
	return s.tradeDao.GetByOrderHash(h)
}

func (s *TradeService) UpdateTradeTxHash(tr *types.Trade, txh common.Hash) error {
	tr.TxHash = txh

	err := s.tradeDao.UpdateByHash(tr.Hash, tr)
	if err != nil {
		logger.Error(err)
		return err
	}

	return nil
}
